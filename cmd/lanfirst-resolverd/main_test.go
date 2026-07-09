package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir string, patterns ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("listen: \"127.0.0.1:5354\"\nentries:\n")
	for _, p := range patterns {
		b.WriteString("  - pattern: \"" + p + "\"\n    target: \"192.168.1.10\"\n    port: 443\n")
	}
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSyncReconciles(t *testing.T) {
	dir := t.TempDir()
	resDir := filepath.Join(dir, "resolver")
	if err := os.MkdirAll(resDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A hand-created (unmanaged) resolver file must never be touched.
	human := filepath.Join(resDir, "human.example")
	if err := os.WriteFile(human, []byte("nameserver 9.9.9.9\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := writeConfig(t, dir, "*.example.com", "app.example.internal")
	if err := sync(resDir, cfg); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Desired files exist, carry the marker, and point at 127.0.0.1:5354.
	for _, d := range []string{"example.com", "app.example.internal"} {
		data, err := os.ReadFile(filepath.Join(resDir, d))
		if err != nil {
			t.Fatalf("expected %s: %v", d, err)
		}
		s := string(data)
		if !strings.HasPrefix(s, marker) || !strings.Contains(s, "nameserver 127.0.0.1") || !strings.Contains(s, "port 5354") {
			t.Fatalf("%s content wrong:\n%s", d, s)
		}
	}

	// Drop one entry → its managed file is removed on next sync.
	cfg = writeConfig(t, dir, "*.example.com")
	if err := sync(resDir, cfg); err != nil {
		t.Fatalf("sync 2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(resDir, "app.example.internal")); !os.IsNotExist(err) {
		t.Fatalf("orphan managed file not removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(resDir, "example.com")); err != nil {
		t.Fatalf("still-desired file missing: %v", err)
	}

	// The human file survives throughout.
	if _, err := os.Stat(human); err != nil {
		t.Fatalf("unmanaged human file was removed: %v", err)
	}
}

func TestCleanupRemovesOnlyManaged(t *testing.T) {
	resDir := t.TempDir()
	managed := filepath.Join(resDir, "corp.io")
	if err := os.WriteFile(managed, []byte(marker+"\nnameserver 127.0.0.1\nport 5354\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	human := filepath.Join(resDir, "keepme")
	if err := os.WriteFile(human, []byte("nameserver 1.1.1.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := removeManaged(resDir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatalf("managed file not removed: %v", err)
	}
	if _, err := os.Stat(human); err != nil {
		t.Fatalf("human file removed by cleanup: %v", err)
	}
}

func TestValidHostname(t *testing.T) {
	ok := []string{"example.com", "app.example.internal", "a-b.c-d.io", "x"}
	bad := []string{"", "..", ".", "a/b", "a..b", ".lead", "trail.", "UP.com", "a_b.com", "a b.com"}
	for _, s := range ok {
		if !validHostname(s) {
			t.Errorf("expected %q valid", s)
		}
	}
	for _, s := range bad {
		if validHostname(s) {
			t.Errorf("expected %q invalid", s)
		}
	}
}

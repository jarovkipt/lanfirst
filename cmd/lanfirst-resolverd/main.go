// Command lanfirst-resolverd is the privileged sync helper: a root LaunchDaemon
// that watches the user's config.yaml and reconciles /etc/resolver/<domain> files
// so the configured domains route to the lanfirstd resolver. It is the only piece
// of lanfirst that needs root, and the only writer of /etc/resolver.
//
// Trust boundary: it consumes a user-writable config but bounds the privilege —
// it only ever writes under /etc/resolver, sanitises each domain to a valid
// hostname before using it as a filename, writes fixed content (127.0.0.1 + a
// numeric port), and deletes only files it itself marked. See ADR-0003.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/jarovkipt/lanfirst/internal/config"
)

// marker gates deletion: resolverd removes only files it wrote. Human-created
// resolver files (without this line) are never touched.
const marker = "# lanfirst-managed"

func main() {
	cfgPath := flag.String("config", config.DefaultPath(), "path to config.yaml")
	// resolverDir defaults to the real macOS path; overridable only for testing.
	// In production it comes from the root-owned plist, never from a user.
	resolverDir := flag.String("resolver-dir", "/etc/resolver", "directory of /etc/resolver files")
	cleanup := flag.Bool("cleanup", false, "remove all lanfirst-managed resolver files and exit")
	flag.Parse()

	if err := os.MkdirAll(*resolverDir, 0o755); err != nil {
		log.Fatalf("ensure %s: %v", *resolverDir, err)
	}

	if *cleanup {
		if err := removeManaged(*resolverDir, nil); err != nil {
			log.Fatalf("cleanup: %v", err)
		}
		log.Printf("removed all lanfirst-managed resolver files")
		return
	}

	reconcile := func() {
		if err := sync(*resolverDir, *cfgPath); err != nil {
			log.Printf("reconcile failed: %v", err)
		}
	}
	reconcile() // initial sync at start

	stop := make(chan struct{})
	log.Printf("lanfirst-resolverd watching %s", *cfgPath)
	if err := config.Watch(*cfgPath, reconcile, stop); err != nil {
		log.Fatalf("watch %s: %v", *cfgPath, err)
	}
}

// sync reconciles resolverDir to the domains in the config: it writes a managed
// file per desired domain and removes managed files no longer desired.
func sync(resolverDir, cfgPath string) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	port, err := cfg.ListenPort()
	if err != nil {
		return err
	}

	desired := map[string]struct{}{}
	for _, d := range cfg.ResolverDomains() {
		if !validHostname(d) {
			log.Printf("skipping invalid domain %q", d)
			continue
		}
		desired[d] = struct{}{}
		if err := writeManaged(resolverDir, d, port); err != nil {
			log.Printf("write %s: %v", d, err)
		}
	}
	return removeManaged(resolverDir, desired)
}

// writeManaged atomically writes a managed resolver file for domain. The path is
// confined to resolverDir and the content is fixed (127.0.0.1 + numeric port).
func writeManaged(resolverDir, domain string, port int) error {
	path := filepath.Join(resolverDir, domain)
	content := fmt.Sprintf("%s\nnameserver 127.0.0.1\nport %d\n", marker, port)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// removeManaged deletes managed resolver files (those containing the marker) whose
// domain is not in keep. A nil keep map removes every managed file (cleanup).
func removeManaged(resolverDir string, keep map[string]struct{}) error {
	ents, err := os.ReadDir(resolverDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, ent := range ents {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if _, ok := keep[name]; ok {
			continue
		}
		path := filepath.Join(resolverDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(string(data), marker) {
			continue // not ours — leave it alone
		}
		if err := os.Remove(path); err != nil {
			log.Printf("remove %s: %v", path, err)
		}
	}
	return nil
}

// validHostname reports whether s is a safe DNS name to use as a filename under
// /etc/resolver: dot-separated labels of [a-z0-9-], each non-empty, no leading or
// trailing dot, and crucially no "." or ".." components (path traversal). The
// config is already lowercased/derived by ResolverDomains, so this is the gate.
func validHostname(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" { // empty label ⇒ leading/trailing/double dot
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

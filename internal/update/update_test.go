package update

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsNewer(t *testing.T) {
	rel := func(tag string) *Release { return &Release{Tag: tag} }
	tests := []struct {
		name    string
		current string
		release string
		want    bool
	}{
		{"dev build never flagged", "", "v1.0.0", false},
		{"invalid current tag", "abc1234-dirty", "v1.0.0", false},
		{"equal", "v0.1.0", "v0.1.0", false},
		{"older release", "v0.2.0", "v0.1.0", false},
		{"newer patch", "v0.1.0", "v0.1.1", true},
		{"newer minor", "v0.1.9", "v0.2.0", true},
		{"newer major", "v0.9.0", "v1.0.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNewer(tt.current, rel(tt.release)); got != tt.want {
				t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.release, got, tt.want)
			}
		})
	}
}

func TestCheck(t *testing.T) {
	latest := `{
		"tag_name": "v0.2.0",
		"assets": [
			{"name": "lanfirst-macos-arm64.tar.gz", "browser_download_url": "https://example.com/lanfirst-macos-arm64.tar.gz"},
			{"name": "checksums.txt", "browser_download_url": "https://example.com/checksums.txt"}
		]
	}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/"+repo+"/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, latest)
	}))
	defer srv.Close()

	orig := apiBase
	apiBase = srv.URL
	defer func() { apiBase = orig }()

	r, err := Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if r.Tag != "v0.2.0" {
		t.Errorf("Tag = %q, want v0.2.0", r.Tag)
	}
	if r.TarballURL != "https://example.com/lanfirst-macos-arm64.tar.gz" {
		t.Errorf("TarballURL = %q", r.TarballURL)
	}
	if r.ChecksumsURL != "https://example.com/checksums.txt" {
		t.Errorf("ChecksumsURL = %q", r.ChecksumsURL)
	}
}

func TestCheckMissingAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "v0.2.0", "assets": []}`)
	}))
	defer srv.Close()

	orig := apiBase
	apiBase = srv.URL
	defer func() { apiBase = orig }()

	if _, err := Check(context.Background()); err == nil {
		t.Fatal("Check succeeded despite missing assets")
	}
}

func TestCheckInvalidTag(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"tag_name": "nightly", "assets": []}`)
	}))
	defer srv.Close()

	orig := apiBase
	apiBase = srv.URL
	defer func() { apiBase = orig }()

	if _, err := Check(context.Background()); err == nil {
		t.Fatal("Check succeeded despite non-semver tag")
	}
}

// Package update implements the self-update channel for lanfirst: it checks
// the project's GitHub Releases for a newer tagged build, downloads and
// checksum-verifies the release tarball, and hands off to the bundled
// installer script (which handles the privileged parts and process restarts).
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"golang.org/x/mod/semver"
)

const (
	repo          = "jarovkipt/lanfirst"
	assetName     = "lanfirst-macos-arm64.tar.gz"
	checksumsName = "checksums.txt"
)

// apiBase is a var so tests can point Check at an httptest server.
var apiBase = "https://api.github.com"

// Release describes the latest published GitHub release.
type Release struct {
	Tag          string // e.g. "v0.2.0"
	TarballURL   string // browser_download_url of the macOS tarball asset
	ChecksumsURL string // browser_download_url of checksums.txt
}

// Check queries the latest GitHub release. The repo is public, so no auth is
// needed; callers should pass a ctx with a short timeout. Returns an error if
// the release exists but is missing the expected assets.
func Check(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/releases/latest", apiBase, repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API: %s", resp.Status)
	}

	var body struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decoding release: %w", err)
	}

	r := &Release{Tag: body.TagName}
	for _, a := range body.Assets {
		switch a.Name {
		case assetName:
			r.TarballURL = a.URL
		case checksumsName:
			r.ChecksumsURL = a.URL
		}
	}
	if !semver.IsValid(r.Tag) {
		return nil, fmt.Errorf("release tag %q is not valid semver", r.Tag)
	}
	if r.TarballURL == "" || r.ChecksumsURL == "" {
		return nil, fmt.Errorf("release %s is missing %s or %s", r.Tag, assetName, checksumsName)
	}
	return r, nil
}

// IsNewer reports whether r is newer than the running build. currentTag == ""
// means a dev/source build, which is never auto-flagged — the manual check
// path handles offering the release channel to dev builds.
func IsNewer(currentTag string, r *Release) bool {
	if currentTag == "" || !semver.IsValid(currentTag) {
		return false
	}
	return semver.Compare(r.Tag, currentTag) > 0
}

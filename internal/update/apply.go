package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

// DownloadAndVerify fetches the release tarball into a fresh temp dir, checks
// its sha256 against the release's checksums.txt, extracts it, and returns the
// extracted lanfirst-vX.Y.Z directory. Nothing outside the temp dir is touched,
// so a failure here leaves the current install intact.
func DownloadAndVerify(ctx context.Context, r *Release) (string, error) {
	dir, err := os.MkdirTemp("", "lanfirst-update-")
	if err != nil {
		return "", err
	}
	cleanup := func() { os.RemoveAll(dir) }

	tarball := filepath.Join(dir, assetName)
	if err := download(ctx, r.TarballURL, tarball); err != nil {
		cleanup()
		return "", fmt.Errorf("downloading %s: %w", assetName, err)
	}

	want, err := expectedChecksum(ctx, r.ChecksumsURL)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("fetching checksums: %w", err)
	}
	got, err := fileSHA256(tarball)
	if err != nil {
		cleanup()
		return "", err
	}
	if got != want {
		cleanup()
		return "", fmt.Errorf("checksum mismatch for %s: got %s, want %s", assetName, got, want)
	}

	// /usr/bin/tar handles the archive; simpler and better-tested than a Go tar walk.
	if out, err := exec.CommandContext(ctx, "/usr/bin/tar", "-xzf", tarball, "-C", dir).CombinedOutput(); err != nil {
		cleanup()
		return "", fmt.Errorf("extracting tarball: %v: %s", err, out)
	}

	stage := filepath.Join(dir, "lanfirst-"+r.Tag)
	if _, err := os.Stat(filepath.Join(stage, "install.sh")); err != nil {
		cleanup()
		return "", fmt.Errorf("tarball missing lanfirst-%s/install.sh: %w", r.Tag, err)
	}
	return stage, nil
}

// LaunchInstaller starts the staged installer in --gui mode, fully detached
// (new session, no inherited pipes) so it survives this process being killed
// by its own `launchctl kickstart -k` step. Output goes to
// ~/Library/Logs/lanfirst-update.log.
func LaunchInstaller(stageDir string) error {
	logPath := filepath.Join(os.Getenv("HOME"), "Library", "Logs", "lanfirst-update.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	cmd := exec.Command("/bin/bash", filepath.Join(stageDir, "install.sh"), "--gui")
	cmd.Dir = stageDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func download(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// expectedChecksum returns the sha256 recorded for assetName in the release's
// checksums.txt (shasum output format: "<hex>  <filename>").
func expectedChecksum(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("no entry for %s in checksums.txt", assetName)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

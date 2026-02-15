package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var (
	ErrDevBuild      = errors.New("cannot update a development build")
	ErrAlreadyLatest = errors.New("already running the latest version")
	ErrChecksum      = errors.New("checksum verification failed")
)

type UpdateInput struct {
	CurrentVersion string
	TargetVersion  string
}

type UpdateProgress struct {
	Stage   string
	Message string
}

func (c *Checker) Update(ctx context.Context, input *UpdateInput, progress func(UpdateProgress)) error {
	if input.CurrentVersion == "(devel)" {
		return ErrDevBuild
	}

	tag := input.TargetVersion
	if tag == "" {
		progress(UpdateProgress{Stage: "check", Message: "Checking for latest version..."})
		result, err := c.Check(ctx, &CheckInput{Version: input.CurrentVersion})
		if err != nil {
			return fmt.Errorf("check for updates: %w", err)
		}
		if !result.UpdateAvailable {
			return ErrAlreadyLatest
		}
		tag = result.LatestVersion
	}

	asset, err := assetName()
	if err != nil {
		return err
	}

	base := strings.TrimRight(c.downloadBaseURL, "/")
	assetURL := fmt.Sprintf("%s/%s/%s/releases/download/%s/%s", base, c.owner, c.repo, tag, asset)
	checksumsURL := fmt.Sprintf("%s/%s/%s/releases/download/%s/checksums.txt", base, c.owner, c.repo, tag)

	progress(UpdateProgress{Stage: "download", Message: fmt.Sprintf("Downloading %s...", tag)})
	archiveData, err := c.downloadFile(ctx, assetURL)
	if err != nil {
		return fmt.Errorf("download archive: %w", err)
	}

	progress(UpdateProgress{Stage: "verify", Message: "Verifying checksum..."})
	checksumsData, err := c.downloadFile(ctx, checksumsURL)
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	checksums := parseChecksums(checksumsData)
	expectedHash, ok := checksums[asset]
	if !ok {
		return fmt.Errorf("no checksum found for %s in checksums.txt", asset)
	}

	if err := verifyChecksum(archiveData, expectedHash); err != nil {
		return err
	}

	progress(UpdateProgress{Stage: "extract", Message: "Extracting binary..."})
	binaryData, err := extractBinary(archiveData, asset)
	if err != nil {
		return fmt.Errorf("extract binary: %w", err)
	}

	progress(UpdateProgress{Stage: "apply", Message: "Applying update..."})
	targetPath, err := c.execPath()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}

	expectedBinaryHash := sha256.Sum256(binaryData)
	if err := applyUpdate(binaryData, targetPath, expectedBinaryHash[:]); err != nil {
		return fmt.Errorf("apply update: %w", err)
	}

	progress(UpdateProgress{Stage: "done", Message: fmt.Sprintf("Updated to %s", tag)})
	return nil
}

func assetName() (string, error) {
	return assetNameFor(runtime.GOOS, runtime.GOARCH)
}

func assetNameFor(goos, goarch string) (string, error) {
	switch goos {
	case "darwin":
		return "mathiz_Darwin_all.tar.gz", nil
	case "linux":
		arch := goarchToRelease(goarch)
		if arch == "" {
			return "", fmt.Errorf("unsupported architecture: %s", goarch)
		}
		return fmt.Sprintf("mathiz_Linux_%s.tar.gz", arch), nil
	case "windows":
		arch := goarchToRelease(goarch)
		if arch == "" {
			return "", fmt.Errorf("unsupported architecture: %s", goarch)
		}
		return fmt.Sprintf("mathiz_Windows_%s.zip", arch), nil
	default:
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}
}

func goarchToRelease(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	case "386":
		return "i386"
	default:
		return ""
	}
}

func (c *Checker) downloadFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}

	return io.ReadAll(resp.Body)
}

func parseChecksums(data []byte) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		result[parts[1]] = parts[0]
	}
	return result
}

func verifyChecksum(data []byte, expectedHex string) error {
	h := sha256.Sum256(data)
	actual := hex.EncodeToString(h[:])
	if actual != expectedHex {
		return fmt.Errorf("%w: expected %s, got %s", ErrChecksum, expectedHex, actual)
	}
	return nil
}

func extractBinary(archiveData []byte, asset string) ([]byte, error) {
	binaryName := "mathiz"
	if strings.HasSuffix(asset, ".zip") {
		binaryName = "mathiz.exe"
		return extractFromZip(archiveData, binaryName)
	}
	return extractFromTarGz(archiveData, binaryName)
}

func extractFromTarGz(data []byte, name string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("open gzip: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read tar: %w", err)
		}
		if filepath.Base(hdr.Name) == name && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

func extractFromZip(data []byte, name string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	for _, f := range r.File {
		if filepath.Base(f.Name) == name {
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer func() { _ = rc.Close() }()
			return io.ReadAll(rc)
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", name)
}

func applyUpdate(binaryData []byte, targetPath string, expectedHash []byte) error {
	info, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("stat target: %w", err)
	}
	originalMode := info.Mode()

	parentDir := filepath.Dir(targetPath)
	tmpDir, err := os.MkdirTemp(parentDir, ".mathiz-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	tmpFile := filepath.Join(tmpDir, "mathiz-new")
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	if _, err := f.Write(binaryData); err != nil {
		_ = f.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// Post-write verification: re-read and compare hash.
	written, err := os.ReadFile(tmpFile)
	if err != nil {
		return fmt.Errorf("re-read temp file: %w", err)
	}
	writtenHash := sha256.Sum256(written)
	if !bytes.Equal(writtenHash[:], expectedHash) {
		return fmt.Errorf("%w: temp file was tampered with after write", ErrChecksum)
	}

	if err := os.Rename(tmpFile, targetPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	if err := os.Chmod(targetPath, originalMode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}

	return nil
}

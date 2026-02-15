package selfupdate

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssetNameFor(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		want    string
		wantErr bool
	}{
		{"darwin amd64", "darwin", "amd64", "mathiz_Darwin_all.tar.gz", false},
		{"darwin arm64", "darwin", "arm64", "mathiz_Darwin_all.tar.gz", false},
		{"linux amd64", "linux", "amd64", "mathiz_Linux_x86_64.tar.gz", false},
		{"linux arm64", "linux", "arm64", "mathiz_Linux_arm64.tar.gz", false},
		{"linux 386", "linux", "386", "mathiz_Linux_i386.tar.gz", false},
		{"windows amd64", "windows", "amd64", "mathiz_Windows_x86_64.zip", false},
		{"windows arm64", "windows", "arm64", "mathiz_Windows_arm64.zip", false},
		{"unsupported os", "freebsd", "amd64", "", true},
		{"unsupported arch", "linux", "mips", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := assetNameFor(tt.goos, tt.goarch)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParseChecksums(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "normal",
			input: "abc123  mathiz_Darwin_all.tar.gz\ndef456  mathiz_Linux_x86_64.tar.gz\n",
			want: map[string]string{
				"mathiz_Darwin_all.tar.gz":    "abc123",
				"mathiz_Linux_x86_64.tar.gz": "def456",
			},
		},
		{
			name:  "empty",
			input: "",
			want:  map[string]string{},
		},
		{
			name:  "malformed lines skipped",
			input: "abc123  file.tar.gz\nbadline\n  \nfoo  bar  baz\nghi789  other.tar.gz\n",
			want: map[string]string{
				"file.tar.gz":  "abc123",
				"other.tar.gz": "ghi789",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseChecksums([]byte(tt.input))
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("hello world")
	h := sha256.Sum256(data)
	correctHex := hex.EncodeToString(h[:])

	t.Run("match", func(t *testing.T) {
		assert.NoError(t, verifyChecksum(data, correctHex))
	})

	t.Run("mismatch", func(t *testing.T) {
		err := verifyChecksum(data, "0000000000000000000000000000000000000000000000000000000000000000")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrChecksum)
	})
}

func TestExtractBinary(t *testing.T) {
	binaryContent := []byte("#!/bin/sh\necho mathiz")

	t.Run("tar.gz", func(t *testing.T) {
		archive := buildTarGz(t, "mathiz", binaryContent)
		got, err := extractBinary(archive, "mathiz_Darwin_all.tar.gz")
		require.NoError(t, err)
		assert.Equal(t, binaryContent, got)
	})

	t.Run("missing binary", func(t *testing.T) {
		archive := buildTarGz(t, "other-file", binaryContent)
		_, err := extractBinary(archive, "mathiz_Darwin_all.tar.gz")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestApplyUpdate(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mathiz")

	// Create original binary with 0755 permissions.
	require.NoError(t, os.WriteFile(target, []byte("old"), 0755))

	newData := []byte("new-binary-content")
	h := sha256.Sum256(newData)

	require.NoError(t, applyUpdate(newData, target, h[:]))

	// Verify content replaced.
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, newData, got)

	// Verify permissions preserved.
	info, err := os.Stat(target)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())
}

func TestUpdate(t *testing.T) {
	binaryContent := []byte("new-mathiz-binary")
	archive := buildTarGz(t, "mathiz", binaryContent)
	archiveHash := sha256.Sum256(archive)
	archiveHex := hex.EncodeToString(archiveHash[:])

	t.Run("happy path", func(t *testing.T) {
		dir := t.TempDir()
		execPath := filepath.Join(dir, "mathiz")
		require.NoError(t, os.WriteFile(execPath, []byte("old"), 0755))

		asset := "mathiz_Darwin_all.tar.gz"
		checksums := fmt.Sprintf("%s  %s\n", archiveHex, asset)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/repos/abhisek/mathiz/releases/latest":
				_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0"}`))
			case r.URL.Path == fmt.Sprintf("/abhisek/mathiz/releases/download/v2.0.0/%s", asset):
				_, _ = w.Write(archive)
			case r.URL.Path == "/abhisek/mathiz/releases/download/v2.0.0/checksums.txt":
				_, _ = w.Write([]byte(checksums))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		checker := NewChecker(
			WithBaseURL(server.URL),
			WithDownloadBaseURL(server.URL),
			withExecPath(func() (string, error) { return execPath, nil }),
		)

		var stages []string
		err := checker.Update(context.Background(), &UpdateInput{CurrentVersion: "v1.0.0"}, func(p UpdateProgress) {
			stages = append(stages, p.Stage)
		})
		require.NoError(t, err)

		// Verify binary was replaced.
		got, err := os.ReadFile(execPath)
		require.NoError(t, err)
		assert.Equal(t, binaryContent, got)

		// Verify all stages were reported.
		assert.Equal(t, []string{"check", "download", "verify", "extract", "apply", "done"}, stages)
	})

	t.Run("dev build", func(t *testing.T) {
		checker := NewChecker()
		err := checker.Update(context.Background(), &UpdateInput{CurrentVersion: "(devel)"}, func(UpdateProgress) {})
		assert.ErrorIs(t, err, ErrDevBuild)
	})

	t.Run("already latest", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"tag_name":"v1.0.0","html_url":"https://example.com/v1.0.0"}`))
		}))
		defer server.Close()

		checker := NewChecker(WithBaseURL(server.URL))
		err := checker.Update(context.Background(), &UpdateInput{CurrentVersion: "v1.0.0"}, func(UpdateProgress) {})
		assert.ErrorIs(t, err, ErrAlreadyLatest)
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		asset := "mathiz_Darwin_all.tar.gz"
		checksums := fmt.Sprintf("%s  %s\n", "0000000000000000000000000000000000000000000000000000000000000000", asset)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/repos/abhisek/mathiz/releases/latest":
				_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0"}`))
			case r.URL.Path == fmt.Sprintf("/abhisek/mathiz/releases/download/v2.0.0/%s", asset):
				_, _ = w.Write(archive)
			case r.URL.Path == "/abhisek/mathiz/releases/download/v2.0.0/checksums.txt":
				_, _ = w.Write([]byte(checksums))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		checker := NewChecker(
			WithBaseURL(server.URL),
			WithDownloadBaseURL(server.URL),
		)
		err := checker.Update(context.Background(), &UpdateInput{CurrentVersion: "v1.0.0"}, func(UpdateProgress) {})
		assert.ErrorIs(t, err, ErrChecksum)
	})

	t.Run("download failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.URL.Path == "/repos/abhisek/mathiz/releases/latest":
				_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","html_url":"https://example.com/v2.0.0"}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		checker := NewChecker(
			WithBaseURL(server.URL),
			WithDownloadBaseURL(server.URL),
		)
		err := checker.Update(context.Background(), &UpdateInput{CurrentVersion: "v1.0.0"}, func(UpdateProgress) {})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "download archive")
	})
}

// buildTarGz creates a tar.gz archive containing a single file.
func buildTarGz(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: name,
		Size: int64(len(content)),
		Mode: 0755,
	}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())
	return buf.Bytes()
}

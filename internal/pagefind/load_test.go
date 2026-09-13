package pagefind

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// currentPlatform is the "GOOS/GOARCH" pair the test binary itself is
// running as - always one of assetSuffix's keys, since docsweb's supported
// platforms (linux/darwin/windows, amd64/arm64) are exactly what CI builds
// and runs this test on.
var currentPlatform = runtime.GOOS + "/" + runtime.GOARCH

// buildArchive returns a gzipped tarball containing a single regular-file
// entry named binName with the given content, mirroring the flat layout of
// a real pagefind release asset.
func buildArchive(t *testing.T, binName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: binName,
		Mode: 0o755,
		Size: int64(len(content)),
	}))
	_, err := tw.Write(content)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func binNameFor(goos string) string {
	if goos == "windows" {
		return "pagefind.exe"
	}
	return "pagefind"
}

func TestEnsureDownloadsVerifiesAndExtracts(t *testing.T) {
	content := []byte("fake pagefind binary contents")
	archive := buildArchive(t, binNameFor(runtime.GOOS), content)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		assert.Equal(t, "/v1.5.2/pagefind-v1.5.2-"+assetSuffix[currentPlatform]+".tar.gz", r.URL.Path)
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = "https://github.com/Pagefind/pagefind/releases/download" })

	cacheDir := t.TempDir()
	opts := Options{Version: "1.5.2", SHA256: map[string]string{currentPlatform: sha256Hex(archive)}}

	bin, err := Ensure(cacheDir, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, requests)

	got, err := os.ReadFile(bin)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(bin)
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&0o100, "binary should be executable")
	}

	// A second call must be a pure cache hit - no further request made.
	bin2, err := Ensure(cacheDir, opts)
	require.NoError(t, err)
	assert.Equal(t, bin, bin2)
	assert.Equal(t, 1, requests)
}

func TestEnsureRejectsChecksumMismatch(t *testing.T) {
	content := []byte("fake pagefind binary contents")
	archive := buildArchive(t, binNameFor(runtime.GOOS), content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = "https://github.com/Pagefind/pagefind/releases/download" })

	cacheDir := t.TempDir()
	opts := Options{Version: "1.5.2", SHA256: map[string]string{currentPlatform: strings.Repeat("0", 64)}}

	_, err := Ensure(cacheDir, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum mismatch")

	// Nothing should have been written to the cache on a failed verification.
	assert.NoDirExists(t, filepath.Join(cacheDir, "pagefind"))
}

func TestEnsureRequiresSHA256ForCurrentPlatform(t *testing.T) {
	requested := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = true
	}))
	defer srv.Close()
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = "https://github.com/Pagefind/pagefind/releases/download" })

	_, err := Ensure(t.TempDir(), Options{Version: "1.5.2", SHA256: map[string]string{"some/other": "deadbeef"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no sha256 provided")
	assert.False(t, requested, "must fail before ever making a network request")
}

func TestEnsureRejectsUnsupportedPlatform(t *testing.T) {
	_, err := ensure(t.TempDir(), "plan9", "amd64", Options{Version: "1.5.2", SHA256: map[string]string{"plan9/amd64": "deadbeef"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported platform")
}

func TestEnsureRejectsMissingBinaryInArchive(t *testing.T) {
	archive := buildArchive(t, "not-pagefind", []byte("nope"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = "https://github.com/Pagefind/pagefind/releases/download" })

	opts := Options{Version: "1.5.2", SHA256: map[string]string{currentPlatform: sha256Hex(archive)}}
	_, err := Ensure(t.TempDir(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no \""+binNameFor(runtime.GOOS)+"\" entry found")
}

func TestEnsureFailsOnServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	releaseBaseURL = srv.URL
	t.Cleanup(func() { releaseBaseURL = "https://github.com/Pagefind/pagefind/releases/download" })

	opts := Options{Version: "9.9.9", SHA256: map[string]string{currentPlatform: "deadbeef"}}
	_, err := Ensure(t.TempDir(), opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status")
}

// Package pagefind downloads and verifies a local copy of the pagefind
// (https://pagefind.app) search-index binary, so a docsweb build can later
// shell out to it to generate a static search index for the site it
// produces.
package pagefind

// @docsweb
// @define pagefind v0.2.0
// @name Pagefind loader
// @summary
// Downloads and sha256-verifies the pagefind release binary matching the
// current OS/architecture into a cache directory, so a build never needs
// pagefind preinstalled, and shells out to it to build a site's search
// index.
// @audience dev
// @changelog
// Adds `Index(bin, siteDir)`, which shells out to an already-`Ensure`d
// pagefind binary against a fully-written static site directory, so a
// build can turn that binary path into an actual search index instead of
// just holding onto it.
// @doc
// # Pagefind loader
//
// `pagefind` depends on no other docsweb package; it only wraps the
// standard library's `net/http`, `archive/tar`, `compress/gzip` and
// `os/exec`.
//
// `Ensure(cacheDir, opts)` makes sure a verified pagefind binary for the
// current OS/architecture is present under `cacheDir`, downloading it first
// if it isn't already cached, and returns the path to the extracted,
// executable binary. `Options.Version` names the pagefind release to fetch
// (e.g. `"1.5.2"`, no leading `v`); `Options.SHA256` maps every
// `"GOOS/GOARCH"` pair docsweb supports (docsweb itself runs on linux,
// macOS and Windows, on both amd64 and arm64) to that platform's expected
// release-asset checksum. Only the current runtime's entry is ever read,
// but a caller is expected to pin every platform its own CI/build fleet
// actually runs on, not just the one it happens to develop on, so a build
// is reproducible regardless of which machine runs it.
//
// The downloaded `.tar.gz` release asset is checksummed against the
// matching `Options.SHA256` entry before its one binary is ever extracted
// from it - a mismatch is a hard error and nothing is written to
// `cacheDir`. A platform with no corresponding entry in `Options.SHA256`,
// or one pagefind doesn't publish a release asset for at all, is also a
// hard error rather than a silent skip, since callers rely on `Ensure`
// actually returning a usable binary path. Once extracted, a repeat call
// for the same `cacheDir`/version/platform is a plain cache hit - no
// network access, no re-verification - so a caller can call `Ensure` on
// every build without worrying about paying the download cost twice.
//
// `Index(bin, siteDir)` runs `bin --site siteDir`, pagefind's own CLI for
// crawling an already-fully-written static site and writing its search
// index plus its own search-runtime JS/CSS under `siteDir/pagefind/`. It
// must be called only after every HTML page a build wants searchable has
// already been written to `siteDir` - pagefind discovers pages by walking
// that directory itself, it is never told about individual files - and
// before anything reads `siteDir/pagefind/` back (e.g. a generated
// "Search" page referencing it). The subprocess's stdout/stderr are
// connected straight through to this process's own, so pagefind's normal
// progress/error output reaches a build's own console unfiltered.
// @docsweb

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// releaseBaseURL is where pagefind publishes its release assets - a package
// var (rather than a constant) purely so tests can point it at a local
// httptest.Server instead of reaching out to the real GitHub.
var releaseBaseURL = "https://github.com/Pagefind/pagefind/releases/download"

// httpClient is used for every release download; a generous but finite
// timeout so a hung connection can't block a build forever.
var httpClient = &http.Client{Timeout: 5 * time.Minute}

// assetSuffix maps every "GOOS/GOARCH" pair docsweb supports to pagefind's
// own release-asset naming (see https://github.com/Pagefind/pagefind/releases).
var assetSuffix = map[string]string{
	"linux/amd64":   "x86_64-unknown-linux-musl",
	"linux/arm64":   "aarch64-unknown-linux-musl",
	"darwin/amd64":  "x86_64-apple-darwin",
	"darwin/arm64":  "aarch64-apple-darwin",
	"windows/amd64": "x86_64-pc-windows-msvc",
	"windows/arm64": "aarch64-pc-windows-msvc",
}

// Options selects which pagefind release Ensure loads and how it is
// verified.
type Options struct {
	// Version is the pagefind release to download, without a leading "v"
	// (e.g. "1.5.2").
	Version string
	// SHA256 maps every "GOOS/GOARCH" pair (e.g. "linux/amd64",
	// "windows/arm64" - the same pair runtime.GOOS/runtime.GOARCH report) a
	// caller cares about to that platform's expected release-asset
	// checksum, as a lowercase hex-encoded sha256 digest.
	SHA256 map[string]string
}

// Ensure makes sure a verified pagefind binary for the current OS/
// architecture, at opts.Version, is present under cacheDir - downloading,
// checksumming and extracting it first if this exact version/platform
// combination hasn't been loaded into cacheDir before - and returns the
// path to the executable binary.
func Ensure(cacheDir string, opts Options) (string, error) {
	return ensure(cacheDir, runtime.GOOS, runtime.GOARCH, opts)
}

func ensure(cacheDir, goos, goarch string, opts Options) (string, error) {
	platform := goos + "/" + goarch
	suffix, ok := assetSuffix[platform]
	if !ok {
		return "", fmt.Errorf("pagefind: unsupported platform %s", platform)
	}
	expected, ok := opts.SHA256[platform]
	if !ok || expected == "" {
		return "", fmt.Errorf("pagefind: no sha256 provided for platform %s", platform)
	}

	binName := "pagefind"
	if goos == "windows" {
		binName = "pagefind.exe"
	}

	destDir := filepath.Join(cacheDir, "pagefind", opts.Version, goos, goarch)
	destBin := filepath.Join(destDir, binName)
	switch _, err := os.Stat(destBin); {
	case err == nil:
		return destBin, nil
	case !os.IsNotExist(err):
		return "", fmt.Errorf("pagefind: checking cached binary %s: %w", destBin, err)
	}

	archiveName := fmt.Sprintf("pagefind-v%s-%s.tar.gz", opts.Version, suffix)
	url := fmt.Sprintf("%s/v%s/%s", releaseBaseURL, opts.Version, archiveName)

	archive, err := downloadArchive(url)
	if err != nil {
		return "", fmt.Errorf("pagefind: downloading %s: %w", url, err)
	}

	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, expected) {
		return "", fmt.Errorf("pagefind: checksum mismatch for %s: got %s, want %s", archiveName, got, expected)
	}

	bin, err := extractBinary(archive, binName)
	if err != nil {
		return "", fmt.Errorf("pagefind: extracting %s from %s: %w", binName, archiveName, err)
	}

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("pagefind: creating %s: %w", destDir, err)
	}
	if err := os.WriteFile(destBin, bin, 0o755); err != nil {
		return "", fmt.Errorf("pagefind: writing %s: %w", destBin, err)
	}
	return destBin, nil
}

func downloadArchive(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// extractBinary reads archive as a gzipped tarball and returns the content
// of the single regular-file entry named binName - every pagefind release
// asset is a flat tarball containing exactly one such entry.
func extractBinary(archive []byte, binName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg || filepath.Base(hdr.Name) != binName {
			continue
		}
		return io.ReadAll(tr)
	}
	return nil, fmt.Errorf("no %q entry found in archive", binName)
}

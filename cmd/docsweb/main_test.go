package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunNoArgs(t *testing.T) {
	err := run(nil)
	assert.ErrorContains(t, err, "expected a command")
}

func TestRunUnknownCommand(t *testing.T) {
	err := run([]string{"frobnicate"})
	assert.ErrorContains(t, err, `unknown command "frobnicate"`)
}

func TestRunBuildMissingConfig(t *testing.T) {
	err := run([]string{"build", "--config", "does/not/exist.yaml", "--out", t.TempDir()})
	assert.Error(t, err)
}

func TestRunBuildEndToEnd(t *testing.T) {
	out := t.TempDir()
	err := run([]string{
		"build",
		"--config", "../../internal/build/testdata/integration/.docsweb.yaml",
		"--out", out,
		// "-search=false": these tests exercise site generation, not the
		// pagefind indexing step, and must stay hermetic - runBuild's
		// default (-search=true) would otherwise download the real
		// pagefind binary over the network on every test run. That step
		// is unit-tested on its own in internal/pagefind.
		"-search=false",
	})
	assert.NoError(t, err)
	assert.FileExists(t, out+"/index.html")
	assert.FileExists(t, out+"/_outdated.html")
	assert.FileExists(t, out+"/integration/app.html")
	assert.FileExists(t, out+"/lib/helper.html")
	assert.FileExists(t, out+"/search.html")
	assert.NoDirExists(t, out+"/pagefind")
}

// TestRunBuildOwnRepo is docsweb's dogfooding smoke test: the project's own
// root .docsweb.yaml (whose "ignore:" rules exclude testdata/ and
// *_test.go) must build cleanly against docsweb's own source tree,
// including README.md itself, a real Markdown-frontend target since its own
// "Markdown files" section was added.
func TestRunBuildOwnRepo(t *testing.T) {
	err := run([]string{
		"build",
		"--config", "../../.docsweb.yaml",
		"--out", t.TempDir(),
		// See TestRunBuildEndToEnd: keep this hermetic, no network access.
		"-search=false",
	})
	assert.NoError(t, err)
}

func TestRunCheckMissingConfig(t *testing.T) {
	err := run([]string{"check", "--config", "does/not/exist.yaml"})
	assert.Error(t, err)
}

func TestRunCheckEndToEnd(t *testing.T) {
	err := run([]string{
		"check",
		"--config", "../../internal/build/testdata/integration/.docsweb.yaml",
	})
	assert.NoError(t, err)
}

// TestRunCheckCatchesBrokenLink confirms "check" fails on data a real build
// would also reject - an unresolvable @link - without ever needing to run a
// build or write anything to disk.
func TestRunCheckCatchesBrokenLink(t *testing.T) {
	err := run([]string{
		"check",
		"--config", "../../internal/check/testdata/links_bad/.docsweb.yaml",
	})
	assert.ErrorContains(t, err, "does not resolve to an existing target")
}

// TestRunCheckOwnRepo mirrors TestRunBuildOwnRepo: docsweb's own root
// .docsweb.yaml must also pass "docsweb check" cleanly.
func TestRunCheckOwnRepo(t *testing.T) {
	err := run([]string{"check", "--config", "../../.docsweb.yaml"})
	assert.NoError(t, err)
}

func TestRunServeMissingConfig(t *testing.T) {
	err := run([]string{"serve", "--config", "does/not/exist.yaml", "--search=false"})
	assert.ErrorContains(t, err, "serve: build")
}

func TestRunServeInvalidAddr(t *testing.T) {
	err := run([]string{
		"serve",
		"--config", "../../internal/build/testdata/integration/.docsweb.yaml",
		"--out", t.TempDir(),
		"--addr", "not-an-address",
		"-search=false",
	})
	assert.ErrorContains(t, err, "serve:")
}

func TestServeDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0o644))

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serveDir(ctx, ln, dir) }()

	base := "http://" + ln.Addr().String()
	resp, err := http.Get(base + "/")
	require.NoError(t, err)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello", string(body))

	resp, err = http.Get(base + "/missing.html")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	cancel()
	assert.NoError(t, <-done)
}

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
	})
	assert.NoError(t, err)
	assert.FileExists(t, out+"/index.html")
	assert.FileExists(t, out+"/_outdated.html")
	assert.FileExists(t, out+"/app.html")
	assert.FileExists(t, out+"/lib/helper.html")
}

// TestRunBuildOwnRepo is docsweb's dogfooding smoke test: the project's own
// root .docsweb.yaml (whose "ignore:" rules exclude testdata/, *_test.go,
// and README.md's own grammar example) must build cleanly against docsweb's
// own source tree.
func TestRunBuildOwnRepo(t *testing.T) {
	err := run([]string{
		"build",
		"--config", "../../.docsweb.yaml",
		"--out", t.TempDir(),
	})
	assert.NoError(t, err)
}

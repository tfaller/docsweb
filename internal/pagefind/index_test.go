package pagefind

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHelperProcess is not a real test - Index's tests below re-exec the
// test binary itself as a stand-in "pagefind" process (the same trick Go's
// own os/exec tests use), so Index can be exercised without a real pagefind
// binary or any network access. A plain `go test` run returns immediately
// since GO_WANT_HELPER_PROCESS is unset.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	if os.Getenv("GO_HELPER_FAIL") == "1" {
		os.Exit(1)
	}
	os.Exit(0)
}

func fakeHelperCommand(fail bool) *exec.Cmd {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcess", "--")
	env := []string{"GO_WANT_HELPER_PROCESS=1"}
	if fail {
		env = append(env, "GO_HELPER_FAIL=1")
	}
	cmd.Env = env
	return cmd
}

func TestIndexRunsBinaryWithSiteFlag(t *testing.T) {
	var gotName string
	var gotArgs []string
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string{}, args...)
		return fakeHelperCommand(false)
	}
	t.Cleanup(func() { execCommand = orig })

	require.NoError(t, Index("/path/to/pagefind", "/out/site"))
	assert.Equal(t, "/path/to/pagefind", gotName)
	assert.Equal(t, []string{"--site", "/out/site"}, gotArgs)
}

func TestIndexSurfacesBinaryFailure(t *testing.T) {
	orig := execCommand
	execCommand = func(name string, args ...string) *exec.Cmd {
		return fakeHelperCommand(true)
	}
	t.Cleanup(func() { execCommand = orig })

	err := Index("/path/to/pagefind", "/out/site")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/out/site")
}

package pagefind

import (
	"fmt"
	"os"
	"os/exec"
)

// execCommand builds the *exec.Cmd Index runs - a package var (like
// releaseBaseURL/httpClient in load.go) purely so tests can swap in a fake
// process instead of a real pagefind binary.
var execCommand = exec.Command

// Index shells out to bin (see Ensure) to build a search index for the
// static site already fully written under siteDir, writing pagefind's own
// index and search-runtime JS/CSS into siteDir/pagefind/. siteDir must
// already contain every HTML page that should be searchable - pagefind
// crawls the directory tree itself, it is never told about individual
// files, so it must run after every page has been written, never before.
func Index(bin, siteDir string) error {
	cmd := execCommand(bin, "--site", siteDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pagefind: indexing %q: %w", siteDir, err)
	}
	return nil
}

package ignore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchBareNameMatchesAnyDepth(t *testing.T) {
	m := Compile([]string{"testdata"})
	assert.True(t, m.Match("testdata", true))
	assert.True(t, m.Match("internal/collect/testdata", true))
	assert.True(t, m.Match("internal/collect/testdata", false))
	assert.False(t, m.Match("internal/collect/nottestdata", true))
}

func TestMatchDirOnlySuffixSkipsFiles(t *testing.T) {
	m := Compile([]string{"dist/"})
	assert.True(t, m.Match("dist", true))
	assert.False(t, m.Match("dist", false))
}

func TestMatchAnchoredLeadingSlashOnlyMatchesRoot(t *testing.T) {
	m := Compile([]string{"/src"})
	assert.True(t, m.Match("src", true))
	assert.False(t, m.Match("nested/src", true))
}

func TestMatchDoubleStarSlashMatchesAnyDepth(t *testing.T) {
	m := Compile([]string{"**/testdata/**"})
	assert.True(t, m.Match("internal/collect/testdata/dup", true))
	assert.True(t, m.Match("internal/collect/testdata/dup/a.go", false))
	// The "testdata" directory itself has nothing after it, so this
	// particular pattern (which requires a trailing segment) doesn't match
	// the directory node itself - only its descendants.
	assert.False(t, m.Match("internal/collect/testdata", true))
}

func TestMatchWildcardStar(t *testing.T) {
	m := Compile([]string{"*.tmp"})
	assert.True(t, m.Match("a.tmp", false))
	assert.True(t, m.Match("nested/a.tmp", false))
	assert.False(t, m.Match("a.tmp.go", false))
}

func TestMatchQuestionMark(t *testing.T) {
	m := Compile([]string{"a?.go"})
	assert.True(t, m.Match("ab.go", false))
	assert.False(t, m.Match("abc.go", false))
}

func TestMatchNegationReincludesLaterPath(t *testing.T) {
	m := Compile([]string{"*.go", "!keep.go"})
	assert.True(t, m.Match("drop.go", false))
	assert.False(t, m.Match("keep.go", false))
}

func TestMatchCommentsAndBlankLinesIgnored(t *testing.T) {
	m := Compile([]string{"", "  ", "# a comment", "dist/"})
	assert.True(t, m.Match("dist", true))
	assert.False(t, m.Match("# a comment", false))
}

func TestMatchLiteralDotIsNotWildcard(t *testing.T) {
	m := Compile([]string{"a.go"})
	assert.True(t, m.Match("a.go", false))
	assert.False(t, m.Match("axgo", false))
}

func TestNilMatcherMatchesNothing(t *testing.T) {
	var m *Matcher
	assert.False(t, m.Match("anything", true))
	assert.False(t, m.Match("anything", false))
}

func TestMatchEmptyPatternListMatchesNothing(t *testing.T) {
	m := Compile(nil)
	assert.False(t, m.Match("anything", true))
}

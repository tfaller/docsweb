package annotation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const mdHappyPath = `<!--
    @docsweb
    @define readme v1.0.0
    @name Docs
    @summary
    Short summary.
    @uses foo@v1.0.0
    @audience dev, user
    @changelog
    Initial add.
-->

# Title

Body paragraph one.

Body paragraph two.
`

func TestParseMarkdownSourceHappyPath(t *testing.T) {
	doc, err := ParseMarkdownSource(mdHappyPath)
	require.NoError(t, err)
	require.NotNil(t, doc)

	assert.Equal(t, "readme", doc.Name)
	assert.Equal(t, "v1.0.0", doc.VersionRaw)
	assert.Equal(t, "Docs", doc.DisplayName)
	assert.Equal(t, "Short summary.", doc.Summary)
	assert.Equal(t, []string{"foo@v1.0.0"}, doc.UsesRaw)
	assert.Equal(t, []string{"dev, user"}, doc.AudienceRaw)
	assert.Equal(t, 3, doc.DefineLine)
	assert.Equal(t, "# Title\n\nBody paragraph one.\n\nBody paragraph two.", doc.Doc)

	require.Len(t, doc.Changelog, 1)
	assert.Equal(t, "Initial add.", doc.Changelog[0].Body)
}

func TestParseMarkdownSourceNoLeadingComment(t *testing.T) {
	src := "# Just a heading\n\nSome content.\n"
	doc, err := ParseMarkdownSource(src)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestParseMarkdownSourceLeadingCommentWithoutDocsweb(t *testing.T) {
	src := "<!-- just a note, not docsweb -->\n\n# Title\n"
	doc, err := ParseMarkdownSource(src)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestParseMarkdownSourceCommentNotAtTop(t *testing.T) {
	src := "Some preamble text.\n\n<!--\n@docsweb\n@define x v1.0.0\n-->\n\nBody.\n"
	doc, err := ParseMarkdownSource(src)
	require.NoError(t, err)
	assert.Nil(t, doc)
}

func TestParseMarkdownSourceMissingDefine(t *testing.T) {
	src := "<!--\n@docsweb\n@name Foo\n-->\n\nBody.\n"
	_, err := ParseMarkdownSource(src)
	assert.ErrorContains(t, err, "@define")
}

func TestParseMarkdownSourceDocTagDisallowed(t *testing.T) {
	src := "<!--\n@docsweb\n@define x v1.0.0\n@doc\nInline doc, not allowed.\n-->\n\nBody.\n"
	_, err := ParseMarkdownSource(src)
	assert.ErrorContains(t, err, "@doc is not allowed")
}

func TestParseMarkdownSourceMultipleBlocksInComment(t *testing.T) {
	src := "<!--\n@docsweb\n@define a v1.0.0\n@docsweb\n@docsweb\n@define b v1.0.0\n@docsweb\n-->\n\nBody.\n"
	_, err := ParseMarkdownSource(src)
	assert.ErrorContains(t, err, "only one @docsweb block")
}

func TestParseMarkdownSourceLeadingBlankLinesTolerated(t *testing.T) {
	src := "\n\n<!--\n@docsweb\n@define x v1.0.0\n-->\n\nBody text.\n"
	doc, err := ParseMarkdownSource(src)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "x", doc.Name)
	assert.Equal(t, "Body text.", doc.Doc)
}

func TestParseMarkdownSourceFenceSuppressesTagRecognition(t *testing.T) {
	src := "<!--\n@docsweb\n@define x v1.0.0\n@summary\nReal summary.\n```\n@uses fake@v1.0.0\n```\nMore summary.\n-->\n\nBody.\n"
	doc, err := ParseMarkdownSource(src)
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Empty(t, doc.UsesRaw)
	assert.Contains(t, doc.Summary, "@uses fake@v1.0.0")
}

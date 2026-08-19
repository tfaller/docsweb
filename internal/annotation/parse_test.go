package annotation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const readmeExample = `
/*
    @docsweb
    @define target v1.0.1
    @name Some cool target (system/module/feature)
    @summary
    Brief summary what this target is about. Optional.
    @uses bla.bla.x@v1.0.0
    @uses xxx@v2.1.0
    @audience dev, tester, user
    @changelog
    @audience user
    Fix types.
    @doc
    This is really important. Document with markdown.
    @docsweb
 */
`

func TestParseSourceReadmeExample(t *testing.T) {
	targets, err := ParseSource(readmeExample)
	require.NoError(t, err)
	require.Len(t, targets, 1)

	tgt := targets[0]
	assert.Equal(t, "target", tgt.Name)
	assert.Equal(t, "v1.0.1", tgt.VersionRaw)
	assert.Equal(t, "Some cool target (system/module/feature)", tgt.DisplayName)
	assert.Equal(t, "Brief summary what this target is about. Optional.", tgt.Summary)
	assert.Equal(t, []string{"bla.bla.x@v1.0.0", "xxx@v2.1.0"}, tgt.UsesRaw)
	assert.Equal(t, []string{"dev, tester, user"}, tgt.AudienceRaw)
	assert.Equal(t, "This is really important. Document with markdown.", tgt.Doc)

	require.Len(t, tgt.Changelog, 1)
	assert.Equal(t, "user", tgt.Changelog[0].AudienceRaw)
	assert.Equal(t, "Fix types.", tgt.Changelog[0].Body)
}

func TestParseSourceMultipleTargetsAndConcatenation(t *testing.T) {
	src := `
/*
    @docsweb
    @define first v1.0.0
    @doc
    First target doc.
*/

/*
    @docsweb
    @doc
    More doc for first target, from a second comment.
    @docsweb
*/

/*
    @docsweb
    @define second v2.0.0
    @doc
    Second target doc.
    @docsweb
*/
`
	targets, err := ParseSource(src)
	require.NoError(t, err)
	require.Len(t, targets, 2)

	assert.Equal(t, "first", targets[0].Name)
	assert.Equal(t, "First target doc.\n\nMore doc for first target, from a second comment.", targets[0].Doc)

	assert.Equal(t, "second", targets[1].Name)
	assert.Equal(t, "Second target doc.", targets[1].Doc)
}

func TestParseSourceFirstBlockWithoutDefineErrors(t *testing.T) {
	src := `
/*
    @docsweb
    @doc
    No define here.
*/
`
	_, err := ParseSource(src)
	assert.Error(t, err)
}

func TestParseSourceFencedCodeBlockHidesTags(t *testing.T) {
	src := "/*\n" +
		"    @docsweb\n" +
		"    @define t v1.0.0\n" +
		"    @doc\n" +
		"    Example usage:\n" +
		"    ```\n" +
		"    @docsweb\n" +
		"    @doc not a real tag\n" +
		"    ```\n" +
		"    Done.\n" +
		"*/\n"

	targets, err := ParseSource(src)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Contains(t, targets[0].Doc, "```\n@docsweb\n@doc not a real tag\n```")
	assert.Contains(t, targets[0].Doc, "Example usage:")
	assert.Contains(t, targets[0].Doc, "Done.")
}

func TestParseSourceLineComments(t *testing.T) {
	src := `
// @docsweb
// @define t v1.0.0
// @doc
// Paragraph one.
//
// Paragraph two.
code.here()
`
	targets, err := ParseSource(src)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "Paragraph one.\n\nParagraph two.", targets[0].Doc)
}

func TestParseSourceHashComments(t *testing.T) {
	src := `
# @docsweb
# @define t v1.0.0
# @name Hash-commented target
# @docsweb
`
	targets, err := ParseSource(src)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "Hash-commented target", targets[0].DisplayName)
}

func TestParseSourceMultipleUsesAndChangelogs(t *testing.T) {
	src := `
/*
    @docsweb
    @define t v1.0.0
    @changelog
    First entry.
    @changelog
    @audience dev
    Second entry, dev only.
    @docsweb
*/
`
	targets, err := ParseSource(src)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.Len(t, targets[0].Changelog, 2)
	assert.Equal(t, "", targets[0].Changelog[0].AudienceRaw)
	assert.Equal(t, "First entry.", targets[0].Changelog[0].Body)
	assert.Equal(t, "dev", targets[0].Changelog[1].AudienceRaw)
	assert.Equal(t, "Second entry, dev only.", targets[0].Changelog[1].Body)
}

func TestParseSourceNoDocswebBlocks(t *testing.T) {
	targets, err := ParseSource("just some code\n// a normal comment\n")
	require.NoError(t, err)
	assert.Empty(t, targets)
}

func TestParseSourceExplicitClosingLeavesTrailingTextUnparsed(t *testing.T) {
	src := `
/*
    @docsweb
    @define t v1.0.0
    @docsweb
    This trailing text is not part of any target.
*/
`
	targets, err := ParseSource(src)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "", targets[0].Doc)
	assert.Equal(t, "", targets[0].DisplayName)
}

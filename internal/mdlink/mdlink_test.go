package mdlink

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/model"
)

// stubResolver is a fake Resolver for tests: it resolves refs found in
// targets to a fixed URL scheme, and treats anchors as valid only if
// declared in anchors.
type stubResolver struct {
	targets map[string]string // TargetRef.Key()@version -> url
	anchors map[string]bool   // TargetRef.Key()@version#anchor -> ok
}

func (s stubResolver) ResolveTarget(ref model.TargetRef) (string, bool) {
	url, ok := s.targets[ref.String()]
	return url, ok
}

func (s stubResolver) HasAnchor(ref model.TargetRef, anchor string) bool {
	return s.anchors[ref.String()+"#"+anchor]
}

func newStubResolver() stubResolver {
	return stubResolver{
		targets: map[string]string{
			"scope.target@v1.0.0": "/scope/target",
			"other@v2.1.0":        "/other",
		},
		anchors: map[string]bool{
			"scope.target@v1.0.0#section": true,
		},
	}
}

func TestCollectAnchors(t *testing.T) {
	md := "Some [Text](@anchor:name) and more Text with [another](@anchor:second)."
	names, err := CollectAnchors(md)
	require.NoError(t, err)
	assert.Equal(t, []string{"name", "second"}, names)
}

func TestCollectAnchorsInvalidName(t *testing.T) {
	_, err := CollectAnchors("[x](@anchor:not valid)")
	assert.Error(t, err)
}

func TestCollectAnchorsDuplicate(t *testing.T) {
	_, err := CollectAnchors("[a](@anchor:dup) ... [b](@anchor:dup)")
	assert.ErrorContains(t, err, "duplicate anchor")
}

func TestCollectAnchorsSkipsFencedCode(t *testing.T) {
	md := "before\n```\n[Text](@anchor:example)\n```\nafter"
	names, err := CollectAnchors(md)
	require.NoError(t, err)
	assert.Empty(t, names)
}

func TestPreprocessAnchorRoundTrip(t *testing.T) {
	out, err := Preprocess("Some [Text](@anchor:name) and more Text", "scope", newStubResolver())
	require.NoError(t, err)
	assert.Equal(t, `Some <a id="name"></a>Text and more Text`, out)
}

func TestPreprocessAnchorInvalidName(t *testing.T) {
	_, err := Preprocess("[Text](@anchor:not-valid)", "scope", newStubResolver())
	assert.Error(t, err)
}

func TestPreprocessLink(t *testing.T) {
	out, err := Preprocess("See [Text](@link:scope.target@v1.0.0)", "scope", newStubResolver())
	require.NoError(t, err)
	assert.Equal(t, "See [Text](/scope/target)", out)
}

func TestPreprocessLinkWithAnchor(t *testing.T) {
	out, err := Preprocess("See [Text](@link:scope.target@v1.0.0#section)", "scope", newStubResolver())
	require.NoError(t, err)
	assert.Equal(t, "See [Text](/scope/target#section)", out)
}

func TestPreprocessLinkDefaultScope(t *testing.T) {
	out, err := Preprocess("See [Text](@link:target@v1.0.0)", "scope", newStubResolver())
	require.NoError(t, err)
	assert.Equal(t, "See [Text](/scope/target)", out)
}

func TestPreprocessLinkUnresolvable(t *testing.T) {
	_, err := Preprocess("See [Text](@link:missing.target@v1.0.0)", "scope", newStubResolver())
	assert.ErrorContains(t, err, "does not resolve to an existing target")
}

func TestPreprocessLinkUnresolvableAnchor(t *testing.T) {
	_, err := Preprocess("See [Text](@link:scope.target@v1.0.0#nope)", "scope", newStubResolver())
	assert.ErrorContains(t, err, `no anchor "nope"`)
}

func TestPreprocessLinkInvalidRef(t *testing.T) {
	_, err := Preprocess("See [Text](@link:not a ref)", "scope", newStubResolver())
	assert.Error(t, err)
}

func TestPreprocessLenientDegradesUnresolvableLinkToPlainLabel(t *testing.T) {
	out := PreprocessLenient("See [Text](@link:missing.target@v1.0.0)", "scope", newStubResolver())
	assert.Equal(t, "See Text", out)
}

func TestPreprocessLenientDegradesUnresolvableAnchorToPlainLabel(t *testing.T) {
	out := PreprocessLenient("See [Text](@link:scope.target@v1.0.0#nope)", "scope", newStubResolver())
	assert.Equal(t, "See Text", out)
}

func TestPreprocessLenientDegradesInvalidRefToPlainLabel(t *testing.T) {
	out := PreprocessLenient("See [Text](@link:not a ref)", "scope", newStubResolver())
	assert.Equal(t, "See Text", out)
}

func TestPreprocessLenientDegradesInvalidAnchorNameToPlainLabel(t *testing.T) {
	out := PreprocessLenient("[Text](@anchor:not-valid)", "scope", newStubResolver())
	assert.Equal(t, "Text", out)
}

func TestPreprocessLenientStillResolvesValidReferences(t *testing.T) {
	out := PreprocessLenient("See [Text](@link:scope.target@v1.0.0#section)", "scope", newStubResolver())
	assert.Equal(t, "See [Text](/scope/target#section)", out)
}

func TestPreprocessPlainLinksUntouched(t *testing.T) {
	md := "Check out [the site](https://example.com) and [an image](./pic.png)."
	out, err := Preprocess(md, "scope", newStubResolver())
	require.NoError(t, err)
	assert.Equal(t, md, out)
}

func TestPreprocessSkipsFencedCode(t *testing.T) {
	md := "before\n```\n[Text](@anchor:name)\n```\nafter"
	out, err := Preprocess(md, "scope", newStubResolver())
	require.NoError(t, err)
	assert.Equal(t, md, out)
}

func TestRenderPlainMarkdown(t *testing.T) {
	md := "# Heading\n\nSome **bold** and _italic_ text.\n\n- one\n- two\n\n```go\ncode here\n```\n"
	html, err := Render(md)
	require.NoError(t, err)
	assert.Contains(t, html, "<h1>Heading</h1>")
	assert.Contains(t, html, "<strong>bold</strong>")
	assert.Contains(t, html, "<em>italic</em>")
	assert.Contains(t, html, "<li>one</li>")
	assert.Contains(t, html, "<pre><code")
}

func TestRenderPreservesInjectedAnchor(t *testing.T) {
	pre, err := Preprocess("Some [Text](@anchor:name) here.", "scope", newStubResolver())
	require.NoError(t, err)
	html, err := Render(pre)
	require.NoError(t, err)
	assert.Contains(t, html, `<a id="name"></a>Text`)
}

func TestRenderDoc(t *testing.T) {
	md := "See [Text](@link:scope.target@v1.0.0#section) and [anchor](@anchor:here)."
	html, err := RenderDoc(md, "scope", newStubResolver())
	require.NoError(t, err)
	assert.Contains(t, html, `<a href="/scope/target#section">Text</a>`)
	assert.Contains(t, html, `<a id="here"></a>anchor`)
}

func TestRenderDocLenientDegradesBrokenLinkInsteadOfFailing(t *testing.T) {
	md := "See [Text](@link:missing.target@v1.0.0) and [anchor](@anchor:here)."
	html, err := RenderDocLenient(md, "scope", newStubResolver())
	require.NoError(t, err)
	assert.Contains(t, html, "See Text and")
	assert.Contains(t, html, `<a id="here"></a>anchor`)
}

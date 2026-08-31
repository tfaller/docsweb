package check

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/collect"
)

func TestCheckLinksAcceptsResolvableLinksAndAnchors(t *testing.T) {
	reg := collect.NewRegistry()
	require.NoError(t, reg.AddScope(collect.Options{Root: os.DirFS("testdata/links_good")}))

	ctx := &context{registry: reg}
	require.NoError(t, checkAnchors(ctx))
	assert.NoError(t, checkLinks(ctx))
}

func TestCheckLinksRejectsUnresolvedTarget(t *testing.T) {
	reg := collect.NewRegistry()
	require.NoError(t, reg.AddScope(collect.Options{Root: os.DirFS("testdata/links_bad")}))

	ctx := &context{registry: reg}
	require.NoError(t, checkAnchors(ctx))

	err := checkLinks(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not resolve to an existing target")
}

func TestCheckLinksRejectsUnresolvedAnchor(t *testing.T) {
	reg := collect.NewRegistry()
	require.NoError(t, reg.AddScope(collect.Options{Root: os.DirFS("testdata/links_good")}))

	// Corrupt the anchor set as if "usage" had never been declared, to
	// exercise the #anchor-specific failure path without a separate fixture.
	ctx := &context{registry: reg}
	require.NoError(t, checkAnchors(ctx))
	delete(ctx.anchors["helper"], "usage")

	err := checkLinks(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no anchor")
}

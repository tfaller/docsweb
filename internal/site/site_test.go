package site_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/build"
	"github.com/tfaller/docsweb/internal/check"
	"github.com/tfaller/docsweb/internal/model"
	"github.com/tfaller/docsweb/internal/site"
)

func v(s string) model.Version {
	ver, err := model.ParseVersion(s)
	if err != nil {
		panic(err)
	}
	return ver
}

// buildResult constructs a small, hand-built build.Result with:
//   - "app" (root scope): uses helper@v1.0.0 (now major-outdated, current
//     v2.0.0) and uses lib2@v1.0.0 (now minor-outdated, current v1.1.0).
//   - "helper" (scope "libs.util"): has a changelog entry scoped to the
//     "dev" audience.
//   - "lib2" (root scope): has a changelog entry with no audience override
//     (whole target audience).
func buildResult() *build.Result {
	app := &model.Target{
		Scope:       "",
		Name:        "app",
		Version:     v("v1.0.0"),
		DisplayName: "Application",
		Summary:     "raw summary md, unused",
		Uses: []model.TargetRef{
			{Scope: "libs.util", Name: "helper", Version: v("v1.0.0")},
			{Scope: "", Name: "lib2", Version: v("v1.0.0")},
		},
		Audiences: []model.Audience{"dev", "user"},
	}
	helper := &model.Target{
		Scope:       "libs.util",
		Name:        "helper",
		Version:     v("v2.0.0"),
		DisplayName: "Helper",
		Changelog: []model.ChangelogEntry{
			{Audiences: []model.Audience{"dev"}, Body: "Rewrote internals."},
		},
	}
	lib2 := &model.Target{
		Scope:   "",
		Name:    "lib2",
		Version: v("v1.1.0"),
		Changelog: []model.ChangelogEntry{
			{Body: "Added a new helper function."},
		},
	}

	targets := []build.RenderedTarget{
		{
			Target:      app,
			SummaryHTML: "<p>App summary HTML</p>",
			DocHTML:     "<p>App documentation body.</p>",
			Uses: []build.UseLink{
				{Label: "libs.util.helper@v1.0.0", URL: "libs/util/helper.html", Found: true},
				{Label: "lib2@v1.0.0", URL: "lib2.html", Found: true},
			},
		},
		{
			Target:  helper,
			DocHTML: "<p>Helper documentation body.</p>",
			ChangelogHTML: []build.ChangelogHTML{
				{Audiences: []model.Audience{"dev"}, HTML: "<p>Rewrote internals.</p>"},
			},
			// "app" @uses "helper" -> the reverse edge.
			UsedBy: []check.UsedByRef{
				{
					User: app.Ref(),
					Use:  model.TargetRef{Scope: "libs.util", Name: "helper", Version: v("v1.0.0")},
				},
			},
			Author: "Alice <alice@example.com>",
		},
		{
			Target:  lib2,
			DocHTML: "<p>Lib2 documentation body.</p>",
			ChangelogHTML: []build.ChangelogHTML{
				{HTML: "<p>Added a new helper function.</p>"},
			},
			// "app" @uses "lib2" -> the reverse edge.
			UsedBy: []check.UsedByRef{
				{
					User: app.Ref(),
					Use:  model.TargetRef{Scope: "", Name: "lib2", Version: v("v1.0.0")},
				},
			},
		},
	}

	issues := []check.UsageIssue{
		{
			User:    app,
			Use:     model.TargetRef{Scope: "libs.util", Name: "helper", Version: v("v1.0.0")},
			Current: v("v2.0.0"),
			Kind:    model.DiffMajor,
		},
		{
			User:    app,
			Use:     model.TargetRef{Scope: "", Name: "lib2", Version: v("v1.0.0")},
			Current: v("v1.1.0"),
			Kind:    model.DiffMinor,
		},
	}

	return &build.Result{Targets: targets, Issues: issues}
}

func TestGenerate_HistoricVersionPages(t *testing.T) {
	target := &model.Target{
		Scope:       "",
		Name:        "app",
		Version:     v("v2.0.0"),
		DisplayName: "Application",
	}
	oldTarget := &model.Target{
		Scope:       "",
		Name:        "app",
		Version:     v("v1.0.0"),
		DisplayName: "Application",
	}

	result := &build.Result{
		Targets: []build.RenderedTarget{
			{
				Target:  target,
				DocHTML: "<p>Current documentation.</p>",
				Versions: []build.VersionLink{
					{Version: v("v2.0.0"), URL: "app.html", Current: true},
					{Version: v("v1.0.0"), URL: "app/v1.0.0.html"},
				},
				History: []build.HistoricVersion{
					{
						Target:  oldTarget,
						DocHTML: "<p>Old documentation.</p>",
						Author:  "Alice <alice@example.com>",
					},
				},
			},
		},
	}

	outDir := t.TempDir()
	require.NoError(t, site.Generate(result, outDir))

	current := readFile(t, filepath.Join(outDir, "app.html"))
	assert.Contains(t, current, "Current documentation.")
	assert.NotContains(t, current, "viewing an old version")
	// Current page's version list: itself as plain text, the old version as
	// a link down into its own subdirectory.
	assert.Contains(t, current, "<strong>v2.0.0 (current)</strong>")
	assert.Contains(t, current, `<a href="app/v1.0.0.html">v1.0.0</a>`)

	old := readFile(t, filepath.Join(outDir, "app", "v1.0.0.html"))
	assert.Contains(t, old, "Old documentation.")
	assert.Contains(t, old, "viewing an old version")
	assert.Contains(t, old, "Last bumped by Alice &lt;alice@example.com&gt;")
	assert.Contains(t, old, "Not tracked for past versions.")
	// Old page's version list: the current version links back up, itself
	// shown as plain text.
	assert.Contains(t, old, `<a href="../app.html">v2.0.0 (current)</a>`)
	assert.Contains(t, old, "<strong>v1.0.0</strong>")
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err, "expected file to exist: %s", path)
	return string(b)
}

func TestGenerate_TargetPages(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	// "app" is root-scoped -> app.html at the output root.
	appPath := filepath.Join(outDir, "app.html")
	app := readFile(t, appPath)
	assert.Contains(t, app, "Application")
	assert.Contains(t, app, "v1.0.0")
	assert.Contains(t, app, "App summary HTML")
	assert.Contains(t, app, "App documentation body.")
	// "app" has no Author (RenderedTarget.Author left empty), so the
	// "Last bumped by" meta fragment must not appear on its page.
	assert.NotContains(t, app, "Last bumped by")
	// Cross-target @uses link to the nested "helper" target.
	assert.Contains(t, app, `href="libs/util/helper.html"`)
	assert.Contains(t, app, "libs.util.helper@v1.0.0")
	// Cross-target @uses link to the root-scoped "lib2" target.
	assert.Contains(t, app, `href="lib2.html"`)
	assert.Contains(t, app, "lib2@v1.0.0")
	// "app" has no dependants of its own.
	assert.Contains(t, app, "No dependants.")

	// Nested-scope target page must land at the nested directory build.TargetURL implies.
	helperPath := filepath.Join(outDir, "libs", "util", "helper.html")
	require.FileExists(t, helperPath)
	helper := readFile(t, helperPath)
	assert.Contains(t, helper, "Helper")
	assert.Contains(t, helper, "v2.0.0")
	assert.Contains(t, helper, "Helper documentation body.")
	assert.Contains(t, helper, "Rewrote internals.")
	assert.Contains(t, helper, "dev") // changelog audience shown
	assert.Contains(t, helper, "Last bumped by Alice &lt;alice@example.com&gt;")
	// "Used by" section: linked back to the dependant's own page.
	assert.Contains(t, helper, "Used by")
	assert.Contains(t, helper, `href="../../app.html"`)
	assert.Contains(t, helper, "app@v1.0.0")

	lib2Path := filepath.Join(outDir, "lib2.html")
	lib2 := readFile(t, lib2Path)
	assert.Contains(t, lib2, "lib2") // falls back to Name since DisplayName is empty
	assert.Contains(t, lib2, "v1.1.0")
	assert.Contains(t, lib2, "Added a new helper function.")
	// No audience override on this changelog entry -> "whole target audience".
	assert.Contains(t, lib2, "whole target audience")
	// "Used by" section: linked back to the dependant's own page.
	assert.Contains(t, lib2, `href="app.html"`)
	assert.Contains(t, lib2, "app@v1.0.0")
}

func TestGenerate_OutdatedPage(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	outdated := readFile(t, filepath.Join(outDir, "_outdated.html"))

	// Major issue: breaking, references helper old v1.0.0 vs current v2.0.0.
	assert.Contains(t, outdated, "app")
	assert.Contains(t, outdated, "libs.util.helper")
	assert.Contains(t, outdated, "v1.0.0")
	assert.Contains(t, outdated, "v2.0.0")
	assert.Contains(t, outdated, "badge-major")
	assert.Contains(t, outdated, "major")

	// Minor issue: informational, references lib2 old v1.0.0 vs current v1.1.0.
	assert.Contains(t, outdated, "lib2")
	assert.Contains(t, outdated, "v1.1.0")
	assert.Contains(t, outdated, "badge-minor")
	assert.Contains(t, outdated, "minor")

	// Both sections are present and distinguishable (not just one list).
	assert.Contains(t, outdated, "Breaking (major)")
	assert.Contains(t, outdated, "Informational (minor)")

	// Assumption #4: current changelog of the referenced target shown as
	// "what's changed since".
	assert.Contains(t, outdated, "Rewrote internals.")
	assert.Contains(t, outdated, "Added a new helper function.")
	assert.Contains(t, outdated, "What's changed since")

	// Links back to both the user and the referenced target's pages.
	assert.Contains(t, outdated, `href="app.html"`)
	assert.Contains(t, outdated, `href="libs/util/helper.html"`)
	assert.Contains(t, outdated, `href="lib2.html"`)
}

func TestGenerate_IndexPage(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	index := readFile(t, filepath.Join(outDir, "index.html"))
	assert.Contains(t, index, `href="app.html"`)
	assert.Contains(t, index, `href="libs/util/helper.html"`)
	assert.Contains(t, index, `href="lib2.html"`)
	assert.Contains(t, index, "Application")
	assert.Contains(t, index, "Helper")
	assert.Contains(t, index, `href="_outdated.html"`)
}

func TestGenerate_NestedScopeDirectoryStructure(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	// build.TargetURL("libs.util", "helper") == "libs/util/helper.html":
	// verify the directories were actually created and nested correctly,
	// and that no flattened/dotted-name file was written instead.
	require.DirExists(t, filepath.Join(outDir, "libs"))
	require.DirExists(t, filepath.Join(outDir, "libs", "util"))
	require.FileExists(t, filepath.Join(outDir, "libs", "util", "helper.html"))
	assert.NoFileExists(t, filepath.Join(outDir, "libs.util.helper.html"))
}

func TestGenerate_NilResult(t *testing.T) {
	err := site.Generate(nil, t.TempDir())
	assert.Error(t, err)
}

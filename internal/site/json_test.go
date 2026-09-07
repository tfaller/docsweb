package site_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tfaller/docsweb/internal/build"
	"github.com/tfaller/docsweb/internal/model"
	"github.com/tfaller/docsweb/internal/site"
)

func readJSON[T any](t *testing.T, path string) T {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err, "expected file to exist: %s", path)
	var v T
	require.NoError(t, json.Unmarshal(b, &v), "invalid JSON in %s", path)
	return v
}

func TestGenerate_TargetJSON(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	app := readJSON[site.Target](t, filepath.Join(outDir, "app.json"))
	assert.Equal(t, "", app.Scope)
	assert.Equal(t, "app", app.Name)
	assert.Equal(t, "Application", app.DisplayName)
	assert.Equal(t, "v1.0.0", app.Version)
	assert.Equal(t, "<p>App summary HTML</p>", app.SummaryHTML)
	assert.Equal(t, "<p>App documentation body.</p>", app.DocHTML)
	assert.False(t, app.Historic)
	assert.Equal(t, "app/versions.json", app.VersionsURL)
	assert.Empty(t, app.Author)
	require.Len(t, app.Uses, 2)
	// Cross-target @uses link carries no resolved URL - see the site
	// package doc on why (resolved via the referenced target's own
	// versions.json instead).
	assert.Equal(t, site.UseLink{Label: "libs.util.helper@v1.0.0", Found: true}, app.Uses[0])
	assert.Equal(t, site.UseLink{Label: "lib2@v1.0.0", Found: true}, app.Uses[1])
	assert.Empty(t, app.UsedBy)

	helperPath := filepath.Join(outDir, "libs", "util", "helper.json")
	require.FileExists(t, helperPath)
	helper := readJSON[site.Target](t, helperPath)
	assert.Equal(t, "libs.util", helper.Scope)
	assert.Equal(t, "helper", helper.Name)
	assert.Equal(t, "v2.0.0", helper.Version)
	assert.Equal(t, "Alice <alice@example.com>", helper.Author)
	require.Len(t, helper.Changelog, 1)
	assert.Equal(t, []model.Audience{"dev"}, helper.Changelog[0].Audiences)
	assert.Equal(t, "Rewrote internals.", helper.Changelog[0].Body)
	assert.Equal(t, "<p>Rewrote internals.</p>", helper.Changelog[0].HTML)
	require.Len(t, helper.UsedBy, 1)
	assert.Equal(t, site.UsedByRef{Label: "app@v1.0.0", URL: "../../app.json"}, helper.UsedBy[0])

	lib2 := readJSON[site.Target](t, filepath.Join(outDir, "lib2.json"))
	assert.Equal(t, "lib2", lib2.Name)
	require.Len(t, lib2.Changelog, 1)
	assert.Empty(t, lib2.Changelog[0].Audiences)
	assert.Equal(t, "Added a new helper function.", lib2.Changelog[0].Body)
}

func TestGenerate_HistoricVersionJSON(t *testing.T) {
	target := &model.Target{Scope: "", Name: "app", Version: v("v2.0.0"), DisplayName: "Application"}
	oldTarget := &model.Target{Scope: "", Name: "app", Version: v("v1.0.0"), DisplayName: "Application"}

	currentCommitTime := time.Date(2026, 3, 2, 10, 30, 0, 0, time.UTC)
	oldCommitTime := time.Date(2026, 1, 5, 8, 0, 0, 0, time.UTC)
	// Full (40-hex-digit-shaped) hashes - build.RenderedTarget/VersionLink/
	// HistoricVersion.CommitHash stores the full hash; only site's HTML
	// rendering derives a short display form from it.
	currentFull := "abcd1234abcd1234abcd1234abcd1234abcd1234"
	oldFull := "def56789def56789def56789def56789def5678"

	result := &build.Result{
		Targets: []build.RenderedTarget{
			{
				Target:     target,
				DocHTML:    "<p>Current documentation.</p>",
				CommitHash: currentFull,
				CommitTime: currentCommitTime,
				Versions: []build.VersionLink{
					{Version: v("v2.0.0"), URL: "app.html", Current: true, CommitHash: currentFull, CommitTime: currentCommitTime},
					{Version: v("v1.0.0"), URL: "app/v1.0.0.html", CommitHash: oldFull, CommitTime: oldCommitTime},
				},
				History: []build.HistoricVersion{
					{
						Target:     oldTarget,
						DocHTML:    "<p>Old documentation.</p>",
						Author:     "Alice <alice@example.com>",
						CommitHash: oldFull,
						CommitTime: oldCommitTime,
					},
				},
			},
		},
	}

	outDir := t.TempDir()
	require.NoError(t, site.Generate(result, outDir))

	current := readJSON[site.Target](t, filepath.Join(outDir, "app.json"))
	assert.False(t, current.Historic)
	// The JSON commitHash is the full hash, not build's short display form.
	assert.Equal(t, currentFull, current.CommitHash)
	require.NotNil(t, current.CommitTime)
	assert.True(t, currentCommitTime.Equal(*current.CommitTime))
	assert.Equal(t, "app/versions.json", current.VersionsURL)

	old := readJSON[site.Target](t, filepath.Join(outDir, "app", "v1.0.0.json"))
	assert.True(t, old.Historic)
	assert.Equal(t, "Alice <alice@example.com>", old.Author)
	assert.Equal(t, oldFull, old.CommitHash)
	require.NotNil(t, old.CommitTime)
	assert.True(t, oldCommitTime.Equal(*old.CommitTime))
	// Historic pages have no "used by" data of their own.
	assert.Empty(t, old.UsedBy)
	// Same shared versions.json as the current page.
	assert.Equal(t, "app/versions.json", old.VersionsURL)

	versions := readJSON[[]site.VersionLink](t, filepath.Join(outDir, "app", "versions.json"))
	require.Len(t, versions, 2)
	assert.Equal(t, site.VersionLink{Version: "v2.0.0", URL: "app.json", Current: true, CommitHash: currentFull, CommitTime: &currentCommitTime}, versions[0])
	assert.Equal(t, "v1.0.0", versions[1].Version)
	assert.Equal(t, "app/v1.0.0.json", versions[1].URL)
	assert.False(t, versions[1].Current)
	assert.Equal(t, oldFull, versions[1].CommitHash)
	require.NotNil(t, versions[1].CommitTime)
	assert.True(t, oldCommitTime.Equal(*versions[1].CommitTime))
}

// TestGenerate_NoCommitMetadataJSON confirms commitHash/commitTime are
// omitted (null/empty, never a zero time.Time) when internal/build left
// them unknown - the JSON-tree equivalent of TestGenerate_NoCommitMetadata.
func TestGenerate_NoCommitMetadataJSON(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	raw, err := os.ReadFile(filepath.Join(outDir, "app.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(raw), `"commitTime"`)
	assert.NotContains(t, string(raw), `"commitHash"`)

	app := readJSON[site.Target](t, filepath.Join(outDir, "app.json"))
	assert.Nil(t, app.CommitTime)
	assert.Empty(t, app.CommitHash)
}

func TestGenerate_IndexJSON(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	index := readJSON[site.IndexPage](t, filepath.Join(outDir, "index.json"))
	assert.Equal(t, "_outdated.json", index.OutdatedURL)

	byScope := map[string][]site.NavLink{}
	for _, g := range index.Groups {
		byScope[g.Scope] = g.Targets
	}
	require.Contains(t, byScope, "")
	require.Contains(t, byScope, "libs.util")

	var appLink, lib2Link *site.NavLink
	for i, l := range byScope[""] {
		switch l.URL {
		case "app.json":
			appLink = &byScope[""][i]
		case "lib2.json":
			lib2Link = &byScope[""][i]
		}
	}
	require.NotNil(t, appLink)
	require.NotNil(t, lib2Link)
	assert.Contains(t, appLink.Label, "Application")
	assert.Equal(t, "libs/util/helper.json", byScope["libs.util"][0].URL)
}

func TestGenerate_OutdatedJSON(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	outdated := readJSON[site.OutdatedPage](t, filepath.Join(outDir, "_outdated.json"))
	require.Len(t, outdated.Major, 1)
	require.Len(t, outdated.Minor, 1)

	major := outdated.Major[0]
	assert.Equal(t, "app", major.UserLabel)
	assert.Equal(t, "app.json", major.UserURL)
	assert.Equal(t, "libs.util.helper@v1.0.0", major.UseLabel)
	assert.Equal(t, "libs/util/helper.json", major.UseURL)
	assert.True(t, major.UseFound)
	assert.Equal(t, "v1.0.0", major.OldVersion)
	assert.Equal(t, "v2.0.0", major.CurrentVersion)
	require.Len(t, major.Changelog, 1)
	assert.Equal(t, "Rewrote internals.", major.Changelog[0].Body)

	minor := outdated.Minor[0]
	assert.Equal(t, "lib2@v1.0.0", minor.UseLabel)
	assert.Equal(t, "lib2.json", minor.UseURL)
	require.Len(t, minor.Changelog, 1)
	assert.Equal(t, "Added a new helper function.", minor.Changelog[0].Body)
}

// TestGenerate_ChangelogJSON exercises the date-indexed changelog feed:
// several versions across two months (one shared day), plus one version
// with no known commit time - which must be skipped entirely rather than
// filed under the zero time.
func TestGenerate_ChangelogJSON(t *testing.T) {
	sept1 := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	sept5 := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	jun3 := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)

	appFull := "aaa11111aaa11111aaa11111aaa11111aaa1111"
	oldFull := "aaa00000aaa00000aaa00000aaa00000aaa0000"
	helperFull := "bbb22222bbb22222bbb22222bbb22222bbb2222"

	appTarget := &model.Target{Scope: "", Name: "app", Version: v("v2.0.0")}
	helperTarget := &model.Target{Scope: "libs.util", Name: "helper", Version: v("v1.1.0")}

	result := &build.Result{
		Targets: []build.RenderedTarget{
			{
				Target:     appTarget,
				CommitHash: appFull,
				CommitTime: sept5,
				Versions: []build.VersionLink{
					{Version: v("v2.0.0"), URL: "app.html", Current: true, CommitHash: appFull, CommitTime: sept5},
					{Version: v("v1.0.0"), URL: "app/v1.0.0.html", CommitHash: oldFull, CommitTime: sept1},
					// No known commit time - must not appear in any shard.
					{Version: v("v0.1.0"), URL: "app/v0.1.0.html"},
				},
			},
			{
				Target:     helperTarget,
				CommitHash: helperFull,
				CommitTime: jun3,
				Versions: []build.VersionLink{
					{Version: v("v1.1.0"), URL: "libs/util/helper.html", Current: true, CommitHash: helperFull, CommitTime: jun3},
				},
			},
		},
	}

	outDir := t.TempDir()
	require.NoError(t, site.Generate(result, outDir))

	index := readJSON[site.ChangelogIndex](t, filepath.Join(outDir, "changelog", "index.json"))
	require.Contains(t, index, "2026")
	// Bare, non-zero-padded month keys.
	assert.ElementsMatch(t, []int{1, 5}, index["2026"]["9"])
	assert.ElementsMatch(t, []int{3}, index["2026"]["6"])
	_, hasZeroPaddedMonth := index["2026"]["09"]
	assert.False(t, hasZeroPaddedMonth)

	sept := readJSON[site.ChangelogShard](t, filepath.Join(outDir, "changelog", "2026-09.json"))
	require.Len(t, sept.Days, 2)
	assert.Equal(t, "2026-09-01", sept.Days[0].Date)
	require.Len(t, sept.Days[0].Versions, 1)
	assert.Equal(t, site.ChangelogVersion{Scope: "", Name: "app", Version: "v1.0.0", CommitHash: oldFull}, sept.Days[0].Versions[0])
	assert.Equal(t, "2026-09-05", sept.Days[1].Date)
	assert.Equal(t, "app", sept.Days[1].Versions[0].Name)
	assert.Equal(t, appFull, sept.Days[1].Versions[0].CommitHash)

	june := readJSON[site.ChangelogShard](t, filepath.Join(outDir, "changelog", "2026-06.json"))
	require.Len(t, june.Days, 1)
	assert.Equal(t, "2026-06-03", june.Days[0].Date)
	assert.Equal(t, site.ChangelogVersion{Scope: "libs.util", Name: "helper", Version: "v1.1.0", CommitHash: helperFull}, june.Days[0].Versions[0])
}

// TestGenerate_ChangelogJSON_NoCommitTimes confirms an empty changelog
// (every version's commit time unknown) still produces a well-formed,
// empty index rather than failing the build.
func TestGenerate_ChangelogJSON_NoCommitTimes(t *testing.T) {
	result := buildResult()
	outDir := t.TempDir()

	require.NoError(t, site.Generate(result, outDir))

	index := readJSON[site.ChangelogIndex](t, filepath.Join(outDir, "changelog", "index.json"))
	assert.Empty(t, index)
	assert.NoFileExists(t, filepath.Join(outDir, "changelog", "2026-09.json"))
}

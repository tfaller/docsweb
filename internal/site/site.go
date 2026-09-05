// Package site renders a build.Result into a static HTML site: one page per
// target, one dedicated "outdated uses" page, and an index page linking
// everything together.
package site

// @docsweb
// @define site v0.10.0
// @name Site
// @summary
// Renders a build.Result into a static HTML site: one page per target
// version (current and past), one dedicated outdated-uses page, and an
// index page linking everything together.
// @uses build@v0.16.0
// @uses model@v0.3.0
// @audience dev
// @changelog
// No behavior change to `site` itself - `@uses` reference bumped to
// [build](@link:build@v0.16.0)'s current version, itself just a `@uses`
// bump following a fix to `check`'s `checkScopes`: a referenced scope
// (local or remote) is now walked with only its own `.docsweb.yaml`
// `ignore:` rules, never the root config's - no behavior change in
// `site` or `build` themselves.
// @doc
// # Site
//
// `Generate` writes three kinds of page under an output directory, all
// sharing one `html/template` page shell:
//
// - **A target page** per known version of every collected target - its
//   current version at [build.TargetURL](@link:build@v0.1.0)'s path, plus
//   one more per `RenderedTarget.History` entry at `build.
//   HistoricTargetURL` - showing display name, version, audiences,
//   rendered summary/doc, its resolved `@uses` list (`build.UseLink`,
//   already pointing at the exact version each reference named), a
//   version-switcher list of every known version, a "Used by" list of
//   every target that depends on it (current pages only - the reverse of
//   `@uses`, computed by `check.ComputeUsedBy`), and its rendered
//   changelog entries.
// - **One [outdated-uses page](@link:build@v0.1.0#outdated)**
//   (`_outdated.html`), grouping every major (breaking) and minor
//   (informational) outdated `@uses` found during the build. Each row
//   links to both the referencing and the referenced target, and shows
//   the referenced target's *current* changelog entries as "what's
//   changed since" - the POC has no synthesized changelog range yet,
//   though `internal/history` now has the raw data such a range could be
//   built from (left for later).
// - **An index page** (`index.html`), grouping every target by scope.
//
// Every page rendered gets an HTML template's default auto-escaping
// except for the pre-rendered pieces that already came out of
// [build](@link:build@v0.1.0) as trusted HTML (`SummaryHTML`, `DocHTML`,
// changelog HTML) - those are inserted verbatim via `template.HTML`.
//
// `Generate` never deletes anything it doesn't itself write, so pointing
// it at a directory that already has unrelated content in it is safe,
// if potentially confusing - picking a dedicated output directory is the
// caller's job.
// @docsweb

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tfaller/docsweb/internal/build"
	"github.com/tfaller/docsweb/internal/check"
	"github.com/tfaller/docsweb/internal/model"
)

// outdatedURL and indexURL are the (root-relative) locations of the two
// site-wide pages, in the same URL scheme build.TargetURL uses for target
// pages.
const (
	outdatedURL = "_outdated.html"
	indexURL    = "index.html"
)

// Generate writes a complete static HTML site for result under outDir.
//
// outDir is created if missing. Existing files are overwritten as needed;
// generation never removes files it does not itself write, so pointing it at
// a directory that already contains unrelated content is safe (if
// potentially confusing - it is the caller's job to pick an output directory
// meant for this purpose).
func Generate(result *build.Result, outDir string) error {
	if result == nil {
		return fmt.Errorf("site: nil result")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("site: creating output directory %q: %w", outDir, err)
	}

	byKey := make(map[string]*build.RenderedTarget, len(result.Targets))
	for i := range result.Targets {
		byKey[result.Targets[i].Target.Key()] = &result.Targets[i]
	}

	for i := range result.Targets {
		if err := writeTargetPage(outDir, &result.Targets[i]); err != nil {
			return err
		}
	}
	if err := writeOutdatedPage(outDir, result, byKey); err != nil {
		return err
	}
	if err := writeIndexPage(outDir, result); err != nil {
		return err
	}
	return nil
}

// -- target page --------------------------------------------------------

type useRef struct {
	Label string
	URL   string
	Found bool
}

type usedByRef struct {
	Label string
	URL   string
}

type versionRow struct {
	Label string
	URL   string
	// Self is true for the row matching the page currently being rendered -
	// shown as plain text rather than a (redundant) self-link.
	Self bool
	// CommitHash and CommitTime are this version's own introducing commit's
	// short hash and formatted committer timestamp, or "" for both if
	// unknown (see build.RenderedTarget.CommitHash).
	CommitHash string
	CommitTime string
}

type changelogItem struct {
	Audiences string
	HTML      template.HTML
}

type targetPageData struct {
	DisplayName string
	Scope       string
	Version     string
	Audiences   string
	Author      string
	// CommitHash and CommitTime describe this page's own version's
	// introducing commit - short hash and formatted committer timestamp, or
	// "" for both if unknown (see build.RenderedTarget.CommitHash).
	CommitHash string
	CommitTime string
	HasSummary bool
	SummaryHTML template.HTML
	DocHTML     template.HTML
	Uses        []useRef
	UsedBy      []usedByRef
	// IsHistoric is true for a past-version page (build.RenderedTarget.
	// History), which has no "used by" data of its own - internal/check
	// only ever computes reverse @uses edges for the current registry.
	IsHistoric bool
	Versions   []versionRow
	Changelog  []changelogItem
}

// writeTargetPage writes rt's current-version page, plus one additional
// page per entry in rt.History - every version internal/history discovered
// gets its own page, all sharing the same version-switcher list.
func writeTargetPage(outDir string, rt *build.RenderedTarget) error {
	t := rt.Target
	url := build.TargetURL(t.Ref())
	data := buildTargetPageData(t, rt.SummaryHTML, rt.DocHTML, rt.ChangelogHTML, rt.Author, rt.CommitHash, rt.CommitTime, rt.Uses, rt.UsedBy, rt.Versions, url, false)
	if err := renderPage(outDir, url, fmt.Sprintf("%s %s", data.DisplayName, data.Version), targetTmpl, data); err != nil {
		return err
	}

	for _, h := range rt.History {
		hurl := build.HistoricTargetURL(h.Target.Ref())
		hdata := buildTargetPageData(h.Target, h.SummaryHTML, h.DocHTML, h.ChangelogHTML, h.Author, h.CommitHash, h.CommitTime, h.Uses, nil, rt.Versions, hurl, true)
		if err := renderPage(outDir, hurl, fmt.Sprintf("%s %s", hdata.DisplayName, hdata.Version), targetTmpl, hdata); err != nil {
			return err
		}
	}
	return nil
}

// buildTargetPageData builds one page's data - the current version's, or one
// historic version's - given that version's own already-rendered/resolved
// pieces and the version list every page of this target shares.
func buildTargetPageData(
	t *model.Target,
	summaryHTML, docHTML string,
	changelog []build.ChangelogHTML,
	author, commitHash string,
	commitTime time.Time,
	uses []build.UseLink,
	usedBy []check.UsedByRef,
	versions []build.VersionLink,
	pageURL string,
	historic bool,
) targetPageData {
	data := targetPageData{
		DisplayName: displayName(t),
		Scope:       scopeLabel(t.Scope),
		Version:     t.Version.String(),
		Audiences:   audienceLabel(t.Audiences),
		Author:      author,
		CommitHash:  commitHash,
		CommitTime:  commitTimeLabel(commitTime),
		DocHTML:     template.HTML(docHTML), //nolint:gosec // pre-rendered, trusted HTML from internal/build
		IsHistoric:  historic,
	}
	if strings.TrimSpace(summaryHTML) != "" {
		data.HasSummary = true
		data.SummaryHTML = template.HTML(summaryHTML) //nolint:gosec // see above
	}

	for _, u := range uses {
		label := u.Label
		if !u.Found {
			// Unreachable for the current page in practice: build's checks
			// hard-error before a Result with an unresolvable current @uses
			// is ever produced. Reachable for a historic page, whose own
			// @uses were never validated the way the current registry's
			// are - see build.resolveUses.
			label += " (unresolved)"
		}
		data.Uses = append(data.Uses, useRef{Label: label, URL: u.URL, Found: u.Found})
	}

	for _, ub := range usedBy {
		data.UsedBy = append(data.UsedBy, usedByRef{
			Label: fmt.Sprintf("%s@%s", ub.User.Key(), ub.User.Version),
			URL:   build.RelLink(pageURL, build.TargetURL(ub.User)),
		})
	}

	for _, v := range versions {
		label := v.Version.String()
		if v.Current {
			label += " (current)"
		}
		row := versionRow{Label: label, CommitHash: v.CommitHash, CommitTime: commitTimeLabel(v.CommitTime)}
		if v.URL == pageURL {
			row.Self = true
		} else {
			row.URL = build.RelLink(pageURL, v.URL)
		}
		data.Versions = append(data.Versions, row)
	}

	for _, c := range changelog {
		data.Changelog = append(data.Changelog, changelogItem{
			Audiences: audienceLabelOrWhole(c.Audiences),
			HTML:      template.HTML(c.HTML), //nolint:gosec // see above
		})
	}

	return data
}

// -- outdated uses page ---------------------------------------------------

type issueRow struct {
	UserLabel      string
	UserURL        string
	UseLabel       string
	UseURL         string
	UseFound       bool
	OldVersion     string
	CurrentVersion string
	Changelog      []changelogItem
}

type outdatedPageData struct {
	Major []issueRow
	Minor []issueRow
}

func writeOutdatedPage(outDir string, result *build.Result, byKey map[string]*build.RenderedTarget) error {
	var data outdatedPageData

	for _, issue := range result.Issues {
		row := issueRow{
			UserLabel:      issue.User.Key(),
			UserURL:        build.RelLink(outdatedURL, build.TargetURL(issue.User.Ref())),
			UseLabel:       fmt.Sprintf("%s@%s", issue.Use.Key(), issue.Use.Version),
			OldVersion:     issue.Use.Version.String(),
			CurrentVersion: issue.Current.String(),
		}

		if used, ok := byKey[issue.Use.Key()]; ok {
			row.UseFound = true
			row.UseURL = build.RelLink(outdatedURL, build.TargetURL(used.Target.Ref()))
			// Per PLAN.md assumption #4: no version history in the POC, so
			// show the referenced target's *current* changelog entries as
			// "what's changed since" rather than a synthesized range.
			for _, c := range used.ChangelogHTML {
				row.Changelog = append(row.Changelog, changelogItem{
					Audiences: audienceLabelOrWhole(c.Audiences),
					HTML:      template.HTML(c.HTML), //nolint:gosec // see above
				})
			}
		}

		switch issue.Kind {
		case model.DiffMajor:
			data.Major = append(data.Major, row)
		case model.DiffMinor:
			data.Minor = append(data.Minor, row)
		case model.DiffNone, model.DiffPatch:
			// build.ResolveUses never emits these kinds as issues.
		}
	}

	return renderPage(outDir, outdatedURL, "Outdated uses", outdatedTmpl, data)
}

// -- index page -------------------------------------------------------------

type navLink struct {
	Label string
	URL   string
}

type scopeGroup struct {
	Scope   string
	Targets []navLink
}

type indexPageData struct {
	Groups       []scopeGroup
	OutdatedLink string
}

func writeIndexPage(outDir string, result *build.Result) error {
	groups := map[string][]navLink{}
	for i := range result.Targets {
		t := result.Targets[i].Target
		label := fmt.Sprintf("%s (%s)", displayName(t), t.Version)
		url := build.TargetURL(t.Ref())
		groups[t.Scope] = append(groups[t.Scope], navLink{Label: label, URL: build.RelLink(indexURL, url)})
	}

	scopes := make([]string, 0, len(groups))
	for s := range groups {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)

	data := indexPageData{OutdatedLink: build.RelLink(indexURL, outdatedURL)}
	for _, s := range scopes {
		links := groups[s]
		sort.Slice(links, func(i, j int) bool { return links[i].Label < links[j].Label })
		data.Groups = append(data.Groups, scopeGroup{Scope: scopeLabel(s), Targets: links})
	}

	return renderPage(outDir, indexURL, "docsweb", indexTmpl, data)
}

// -- shared helpers -----------------------------------------------------

func displayName(t *model.Target) string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	return t.Name
}

func scopeLabel(scope string) string {
	if scope == "" {
		return "(root)"
	}
	return scope
}

func audienceLabel(auds []model.Audience) string {
	if len(auds) == 0 {
		return "all"
	}
	return joinAudiences(auds)
}

func audienceLabelOrWhole(auds []model.Audience) string {
	if len(auds) == 0 {
		return "whole target audience"
	}
	return joinAudiences(auds)
}

// commitTimeLabel formats a commit's committer timestamp for display, or ""
// for the zero time - build.RenderedTarget.CommitTime's best-effort-unknown
// case.
func commitTimeLabel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func joinAudiences(auds []model.Audience) string {
	strs := make([]string, len(auds))
	for i, a := range auds {
		strs[i] = string(a)
	}
	return strings.Join(strs, ", ")
}

// renderPage executes tmpl with data to produce a page's body content, wraps
// it in the common page shell, and writes the result to outDir/relURL
// (creating any needed subdirectories for nested scopes).
func renderPage(outDir, relURL, title string, tmpl *template.Template, data any) error {
	var body bytes.Buffer
	if err := tmpl.Execute(&body, data); err != nil {
		return fmt.Errorf("site: rendering %s: %w", relURL, err)
	}

	sd := shellData{
		Title:        title,
		Body:         template.HTML(body.String()), //nolint:gosec // body built from our own templates
		IndexLink:    build.RelLink(relURL, indexURL),
		OutdatedLink: build.RelLink(relURL, outdatedURL),
	}
	var page bytes.Buffer
	if err := shellTmpl.ExecuteTemplate(&page, "shell", sd); err != nil {
		return fmt.Errorf("site: wrapping %s: %w", relURL, err)
	}

	dest := filepath.Join(outDir, filepath.FromSlash(relURL))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("site: creating directory for %s: %w", relURL, err)
	}
	if err := os.WriteFile(dest, page.Bytes(), 0o644); err != nil {
		return fmt.Errorf("site: writing %s: %w", relURL, err)
	}
	return nil
}

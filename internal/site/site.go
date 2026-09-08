// Package site renders a build.Result into a static site: one HTML page
// (plus a parallel JSON file) per target, one dedicated "outdated uses"
// page, an index page linking everything together, and a dynamic,
// client-rendered changelog tab backed by the date-indexed changelog feed.
package site

// @docsweb
// @define site v0.16.0
// @name Site
// @summary
// Renders a build.Result into a static site: one HTML page (plus a
// parallel JSON file) per target version, one dedicated outdated-uses page,
// an index page linking everything together, and a dynamic "Changelog" tab.
// @uses build@v0.18.0
// @uses model@v0.3.0
// @audience dev
// @changelog
// A new "Changelog" tab (`changelog.html`, linked from every page's shared
// nav bar) renders the date-indexed changelog feed as an interactive,
// client-side page instead of leaving it as JSON-only data: a date range,
// a major/minor+/patch+ change-level threshold, and a scope filter, with
// whole-days-only pagination (loads until at least 100 changes are
// visible, a "Load more" button beyond that) - see `changelogTmpl` in
// `templates.go`. It loads everything lazily and, since opening a
// generated site directly off disk (`file://`) is a normal way to browse
// it and browsers block `fetch`/`XHR` outright there ("CORS request not
// http" - confirmed by actually driving the rendered site in a browser
// under both `file://` and a real HTTP server, not just by reading the
// code), it loads every JSON file via a classic JSONP `<script src>`
// instead: `writeJSONFile` now writes every JSON file's data a second
// time as an executable `"<url>.js"` sibling calling a fixed global
// `docsweb_jsonp(url, data)` (`writeJSONPFile`) - the plain `.json` files
// are unchanged, still the documented external API.
//
// `ChangelogVersion` (`json.go`) gained `Kind` ("major"/"minor"/"patch",
// via `model.Diff` against the immediately preceding known version) so the
// change-level filter needs no extra fetch. Every version's own JSON file
// - `<target>/vX.Y.Z.json` - now also exists for the *current* version,
// not just historic ones (identical content to `<target>.json`): every
// version, current or historic alike, is addressable purely from
// scope+name+version, so the changelog tab (or any other client) never
// needs a separate lookup (e.g. `versions.json`) just to tell them apart.
// @doc
// # Site
//
// `Generate` writes three kinds of HTML page under an output directory, all
// sharing one `html/template` page shell, plus one dynamic app page (the
// changelog tab, see below):
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
// Every static page shares the nav bar's "Changelog" link, which opens
// **the changelog tab** (`changelog.html`) - the one page whose content
// isn't pre-rendered by `Generate` at all, only its own static shell.
// Everything it shows comes from an inline script loading the JSON data
// API described below, at the time a reader opens it: `changelog/index.
// json`, then only the month shards a chosen (or default) date range
// needs, then only the individual target version JSON files for the
// changelog entries actually rendered on screen. See `changelogTmpl` in
// `templates.go` for the full client-side implementation - the date range,
// change-level, and scope filters, and the whole-days-only "at least 100,
// then load more on request" pagination rule. It loads that JSON via a
// `<script src>` tag calling `docsweb_jsonp`, never `fetch`, so the tab
// (and, by the same mechanism, any other consumer of the JSON API) works
// identically whether the site is served over HTTP or opened straight off
// disk as `file://` - see "JSON data API" below.
//
// Every page rendered gets an HTML template's default auto-escaping
// except for the pre-rendered pieces that already came out of
// [build](@link:build@v0.1.0) as trusted HTML (`SummaryHTML`, `DocHTML`,
// changelog HTML) - those are inserted verbatim via `template.HTML`.
//
// ## JSON data API
//
// Alongside every HTML page above, `Generate` writes a same-shaped JSON
// file - one JSON value per fetch, sized for a client to load only the
// piece it needs rather than a single monolithic document:
//
// - Every target page's JSON sibling (`Target`) mirrors that page's data,
//   plus both the raw Markdown and the rendered HTML for its
//   summary/doc/changelog, so a client can diff or search the source text
//   without stripping HTML back out of it.
// - **`<target>/versions.json`** (`VersionLink`) lists every version of
//   that target, current and historic - the JSON tree's stand-in for the
//   HTML version-switcher list, since there's no single page to embed one
//   in.
// - **`index.json`** (`IndexPage`) and **`_outdated.json`**
//   (`OutdatedPage`) mirror `index.html`/`_outdated.html`.
// - **`changelog/<YYYY-MM>.json`** (`ChangelogShard`) lists every version
//   introduced that month, grouped by day - each carrying a `Kind`
//   ("major"/"minor"/"patch", against the immediately preceding known
//   version) so a client can filter by change severity without fetching
//   every entry's own target JSON, and its own `URL` (unlike every other
//   cross-reference here, see below) - and **`changelog/index.json`**
//   (`ChangelogIndex`) sparsely maps which year/month/day combinations
//   actually have an entry, so a client knows what to fetch without
//   probing every month.
//
// A reference to another target read from *within* some other target's own
// JSON (`UseLink`) deliberately carries no resolved URL: only that
// referenced target's own `versions.json` can tell a current-version
// page's bare URL apart from a historic one's, so a client already holding
// that target's data looks it up there rather than it being duplicated (and
// needing RelLink-style adjustment per referencing page) at every
// reference site. `ChangelogVersion` is the one exception: since it's the
// very first and only thing the changelog tab loads about that specific
// version, and every root-relative URL already means the same thing from
// wherever `changelog.html` (always the site root) fetched it, its own
// `URL` is included directly - avoiding a second round trip per entry is
// worth more here than the general "look it up once" rule. This is safe
// because `changelog/<YYYY-MM>.json` is never reused across builds: it's
// always fully regenerated from that build's own version list, so its URLs
// always match that same build's actual file layout.
//
// Every JSON file above (`writeJSONFile`, `json.go`) gets a second,
// executable `"<url>.js"` sibling calling a fixed global `docsweb_jsonp
// (url, data)` function with its own root-relative url and exactly the
// same data - a classic JSONP `<script src>` load, unaffected by the CORS
// restriction that blocks `fetch`/`XHR` outright when a page is opened as
// `file://` instead of served over HTTP. The changelog tab loads
// everything this way; the plain `.json` files are unaffected and remain
// the documented API for any other consumer.
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
	outdatedURL  = "_outdated.html"
	indexURL     = "index.html"
	changelogURL = "changelog.html"
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
		if err := writeTargetJSON(outDir, &result.Targets[i]); err != nil {
			return err
		}
	}
	if err := writeOutdatedPage(outDir, result, byKey); err != nil {
		return err
	}
	if err := writeOutdatedJSON(outDir, result, byKey); err != nil {
		return err
	}
	if err := writeIndexPage(outDir, result); err != nil {
		return err
	}
	if err := writeIndexJSON(outDir, result); err != nil {
		return err
	}
	if err := writeChangelogJSON(outDir, result); err != nil {
		return err
	}
	if err := writeChangelogPage(outDir); err != nil {
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
	CommitHash  string
	CommitTime  string
	HasSummary  bool
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
		CommitHash:  shortHash(commitHash),
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
		row := versionRow{Label: label, CommitHash: shortHash(v.CommitHash), CommitTime: commitTimeLabel(v.CommitTime)}
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

// -- changelog page -------------------------------------------------------

// writeChangelogPage writes the site-wide changelog tab: a static shell
// around a client-side app that lazy-loads changelog/index.json,
// changelog/<YYYY-MM>.json shards, and individual target version JSON files
// (for changelog text + a link to the full version doc) directly out of the
// JSON data API writeChangelogJSON/writeTargetJSON already produce - no
// server-side data of its own to pass in, unlike every other page.
func writeChangelogPage(outDir string) error {
	return renderPage(outDir, changelogURL, "Changelog", changelogTmpl, nil)
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

// shortHash renders build.RenderedTarget.CommitHash's full (40-hex-digit)
// hash in the short, display-friendly form HTML pages show, or "" if it's
// unknown.
func shortHash(full string) string {
	if len(full) > 7 {
		return full[:7]
	}
	return full
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
		Title:         title,
		Body:          template.HTML(body.String()), //nolint:gosec // body built from our own templates
		IndexLink:     build.RelLink(relURL, indexURL),
		OutdatedLink:  build.RelLink(relURL, outdatedURL),
		ChangelogLink: build.RelLink(relURL, changelogURL),
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

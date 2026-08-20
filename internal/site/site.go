// Package site renders a build.Result into a static HTML site: one page per
// target, one dedicated "outdated uses" page, and an index page linking
// everything together.
package site

// @docsweb
// @define site v0.1.0
// @name Site
// @summary
// Renders a build.Result into a static HTML site: one page per target,
// one dedicated outdated-uses page, and an index page linking everything
// together.
// @uses build@v0.1.0
// @uses model@v0.1.0
// @audience dev
// @changelog
// Initial documentation.
// @doc
// # Site
//
// `Generate` writes three kinds of page under an output directory, all
// sharing one `html/template` page shell:
//
// - **A target page** per collected target, at
//   [build.TargetURL](@link:build@v0.1.0)'s path: display name, version,
//   audiences, rendered summary/doc, its resolved `@uses` list, and its
//   rendered changelog entries.
// - **One [outdated-uses page](@link:build@v0.1.0#outdated)**
//   (`_outdated.html`), grouping every major (breaking) and minor
//   (informational) outdated `@uses` found during the build. Each row
//   links to both the referencing and the referenced target, and shows
//   the referenced target's *current* changelog entries as "what's
//   changed since" - the POC has no version history to synthesize an
//   exact range from.
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

	"github.com/tfaller/docsweb/internal/build"
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
		if err := writeTargetPage(outDir, &result.Targets[i], byKey); err != nil {
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

type changelogItem struct {
	Audiences string
	HTML      template.HTML
}

type targetPageData struct {
	DisplayName string
	Scope       string
	Version     string
	Audiences   string
	HasSummary  bool
	SummaryHTML template.HTML
	DocHTML     template.HTML
	Uses        []useRef
	Changelog   []changelogItem
}

func writeTargetPage(outDir string, rt *build.RenderedTarget, byKey map[string]*build.RenderedTarget) error {
	t := rt.Target
	url := build.TargetURL(t.Ref())

	data := targetPageData{
		DisplayName: displayName(t),
		Scope:       scopeLabel(t.Scope),
		Version:     t.Version.String(),
		Audiences:   audienceLabel(t.Audiences),
		DocHTML:     template.HTML(rt.DocHTML), //nolint:gosec // pre-rendered, trusted HTML from internal/build
	}
	if strings.TrimSpace(rt.SummaryHTML) != "" {
		data.HasSummary = true
		data.SummaryHTML = template.HTML(rt.SummaryHTML) //nolint:gosec // see above
	}

	for _, use := range t.Uses {
		ur := useRef{Label: fmt.Sprintf("%s@%s", use.Key(), use.Version)}
		if used, ok := byKey[use.Key()]; ok {
			ur.Found = true
			ur.URL = relLink(url, build.TargetURL(used.Target.Ref()))
		} else {
			// Unreachable in practice: build.Run's ResolveUses hard-errors
			// before a Result with an unresolvable @uses is ever produced.
			// Handled defensively here only so a hand-built Result (as in
			// this package's own tests) can't panic.
			ur.Label += " (unresolved)"
		}
		data.Uses = append(data.Uses, ur)
	}

	for _, c := range rt.ChangelogHTML {
		data.Changelog = append(data.Changelog, changelogItem{
			Audiences: audienceLabelOrWhole(c.Audiences),
			HTML:      template.HTML(c.HTML), //nolint:gosec // see above
		})
	}

	return renderPage(outDir, url, fmt.Sprintf("%s %s", data.DisplayName, data.Version), targetTmpl, data)
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
			UserURL:        relLink(outdatedURL, build.TargetURL(issue.User.Ref())),
			UseLabel:       fmt.Sprintf("%s@%s", issue.Use.Key(), issue.Use.Version),
			OldVersion:     issue.Use.Version.String(),
			CurrentVersion: issue.Current.String(),
		}

		if used, ok := byKey[issue.Use.Key()]; ok {
			row.UseFound = true
			row.UseURL = relLink(outdatedURL, build.TargetURL(used.Target.Ref()))
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
		groups[t.Scope] = append(groups[t.Scope], navLink{Label: label, URL: relLink(indexURL, url)})
	}

	scopes := make([]string, 0, len(groups))
	for s := range groups {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)

	data := indexPageData{OutdatedLink: relLink(indexURL, outdatedURL)}
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

func joinAudiences(auds []model.Audience) string {
	strs := make([]string, len(auds))
	for i, a := range auds {
		strs[i] = string(a)
	}
	return strings.Join(strs, ", ")
}

// relLink computes a relative link from the page at fromURL to the page at
// toURL, where both are root-relative URLs in build.TargetURL's scheme
// (slash-separated, no "." or ".." segments). The result is simply
// "../" repeated once per directory level fromURL is nested under, followed
// by toURL.
func relLink(fromURL, toURL string) string {
	depth := strings.Count(fromURL, "/")
	return strings.Repeat("../", depth) + toURL
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
		IndexLink:    relLink(relURL, indexURL),
		OutdatedLink: relLink(relURL, outdatedURL),
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

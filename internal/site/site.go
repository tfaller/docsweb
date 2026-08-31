// Package site renders a build.Result into a static HTML site: one page per
// target, one dedicated "outdated uses" page, and an index page linking
// everything together.
package site

// @docsweb
// @define site v0.3.2
// @name Site
// @summary
// Renders a build.Result into a static HTML site: one page per target,
// one dedicated outdated-uses page, and an index page linking everything
// together.
// @uses build@v0.9.0
// @uses model@v0.3.0
// @audience dev
// @changelog
// No behavior change - `@uses` reference bumped to
// [build](@link:build@v0.9.0)'s current version following its move to
// sourcing a remote scope's git-blame `Repository` from `check.Result`
// instead of `vcs.Open`, which `Generate` here doesn't touch (it only reads
// the already-populated `RenderedTarget.Author` field).
// @doc
// # Site
//
// `Generate` writes three kinds of page under an output directory, all
// sharing one `html/template` page shell:
//
// - **A target page** per collected target, at
//   [build.TargetURL](@link:build@v0.1.0)'s path: display name, version,
//   audiences, rendered summary/doc, its resolved `@uses` list, a "Used
//   by" list of every target that depends on it (the reverse of `@uses`,
//   computed by `check.ComputeUsedBy` - no separate annotation needed),
//   and its rendered changelog entries.
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

type usedByRef struct {
	Label string
	URL   string
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
	HasSummary  bool
	SummaryHTML template.HTML
	DocHTML     template.HTML
	Uses        []useRef
	UsedBy      []usedByRef
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
		Author:      rt.Author,
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
			ur.URL = build.RelLink(url, build.TargetURL(used.Target.Ref()))
		} else {
			// Unreachable in practice: build.Run's ResolveUses hard-errors
			// before a Result with an unresolvable @uses is ever produced.
			// Handled defensively here only so a hand-built Result (as in
			// this package's own tests) can't panic.
			ur.Label += " (unresolved)"
		}
		data.Uses = append(data.Uses, ur)
	}

	for _, ub := range rt.UsedBy {
		data.UsedBy = append(data.UsedBy, usedByRef{
			Label: fmt.Sprintf("%s@%s", ub.User.Key(), ub.User.Version),
			URL:   build.RelLink(url, build.TargetURL(ub.User)),
		})
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

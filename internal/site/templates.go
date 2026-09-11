package site

import "html/template"

// shellData is the common page-shell template's data: every page (target,
// outdated, index) is rendered into a body first, then wrapped in this
// shell so nav/CSS live in exactly one place.
type shellData struct {
	Title         string
	Body          template.HTML
	IndexLink     string
	OutdatedLink  string
	ChangelogLink string
}

var shellTmpl = template.Must(template.New("shell").Parse(`{{define "shell"}}<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
  :root { color-scheme: light dark; }
  body {
    font-family: system-ui, -apple-system, "Segoe UI", sans-serif;
    max-width: 60rem;
    margin: 2rem auto;
    padding: 0 1rem 4rem;
    line-height: 1.55;
  }
  header.site-nav {
    display: flex;
    gap: 1.25rem;
    margin-bottom: 2rem;
    padding-bottom: 0.6rem;
    border-bottom: 1px solid #8888;
    font-size: 0.95rem;
  }
  h1 { margin-bottom: 0.2rem; }
  h2 { margin-top: 2rem; }
  .meta { color: #777; margin-bottom: 1.5rem; font-size: 0.95rem; }
  .badge {
    display: inline-block;
    padding: 0.15rem 0.55rem;
    border-radius: 0.3rem;
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: 0.03em;
    text-transform: uppercase;
    margin-right: 0.5rem;
  }
  .badge-major { background: #fee2e2; color: #991b1b; }
  .badge-minor { background: #dbeafe; color: #1e40af; }
  .historic-banner {
    background: #fef3c7;
    color: #92400e;
    padding: 0.5rem 0.9rem;
    border-radius: 0.35rem;
    font-size: 0.9rem;
    margin-bottom: 1.5rem;
  }
  section { margin-bottom: 2rem; }
  ul.plain { list-style: none; padding-left: 0; }
  ul.plain li { margin-bottom: 0.35rem; }
  .issue {
    border: 1px solid #8886;
    border-radius: 0.4rem;
    padding: 0.9rem 1rem;
    margin-bottom: 1rem;
  }
  .changelog-entry {
    border-left: 3px solid #8886;
    padding-left: 0.9rem;
    margin: 0.8rem 0;
  }
  .changelog-audience {
    font-weight: 600;
    color: #666;
    font-size: 0.85rem;
    text-transform: uppercase;
    letter-spacing: 0.03em;
    margin-bottom: 0.2rem;
  }
  .changed-since {
    margin-top: 0.6rem;
    padding-top: 0.6rem;
    border-top: 1px dashed #8886;
  }
  .changed-since > em { font-size: 0.85rem; color: #666; }
  code { background: #8882; padding: 0.05rem 0.35rem; border-radius: 0.25rem; }
</style>
</head>
<body>
<header class="site-nav">
  <a href="{{.IndexLink}}">Index</a>
  <a href="{{.OutdatedLink}}">Outdated uses</a>
  <a href="{{.ChangelogLink}}">Changelog</a>
</header>
{{.Body}}
</body>
</html>
{{end}}`))

var targetTmpl = template.Must(template.New("target").Parse(`
<h1>{{.DisplayName}}</h1>
<p class="meta">Scope: {{.Scope}} &middot; Version: {{.Version}} &middot; Audience: {{.Audiences}}{{if .Author}} &middot; Last bumped by {{.Author}}{{end}}{{if .CommitTime}} &middot; Committed {{.CommitTime}}{{end}}{{if .CommitHash}} &middot; Commit <code>{{.CommitHash}}</code>{{end}}</p>
{{if .IsHistoric}}<p class="historic-banner">You are viewing an old version of this documentation.</p>{{end}}

{{if gt (len .Versions) 1}}<section class="versions">
<h2>Versions</h2>
<ul class="plain">
{{range .Versions}}<li>{{if .Self}}<strong>{{.Label}}</strong>{{else}}<a href="{{.URL}}">{{.Label}}</a>{{end}}{{if or .CommitTime .CommitHash}} &mdash;{{if .CommitTime}} {{.CommitTime}}{{end}}{{if .CommitHash}} <code>{{.CommitHash}}</code>{{end}}{{end}}</li>
{{end}}</ul>
</section>{{end}}

{{if .HasSummary}}<section class="summary">{{.SummaryHTML}}</section>{{end}}

<section class="doc">{{.DocHTML}}</section>

<section class="uses">
<h2>Uses</h2>
{{if .Uses}}<ul class="plain">
{{range .Uses}}<li>{{if .Found}}<a href="{{.URL}}">{{.Label}}</a>{{else}}{{.Label}}{{end}}</li>
{{end}}</ul>
{{else}}<p><em>No @uses references.</em></p>{{end}}
</section>

<section class="used-by">
<h2>Used by</h2>
{{if .IsHistoric}}<p><em>Not tracked for past versions.</em></p>
{{else}}{{if .UsedBy}}<ul class="plain">
{{range .UsedBy}}<li><a href="{{.URL}}">{{.Label}}</a></li>
{{end}}</ul>
{{else}}<p><em>No dependants.</em></p>{{end}}{{end}}
</section>

<section class="changelog">
<h2>Changelog</h2>
{{if .Changelog}}
{{range .Changelog}}<div class="changelog-entry"><div class="changelog-audience">{{.Audiences}}</div>{{.HTML}}</div>
{{end}}
{{else}}<p><em>No changelog entries.</em></p>{{end}}
</section>
`))

var outdatedTmpl = template.Must(template.New("outdated").Parse(`
<h1>Outdated uses</h1>
<p>Every <code>@uses</code> reference whose referenced target has moved on to a newer version.
Major version changes are breaking; minor version changes are informational only.</p>

<section class="major">
<h2>Breaking (major)</h2>
{{if .Major}}
{{range .Major}}<div class="issue">
  <p><span class="badge badge-major">major</span>
  <strong>{{.UserLabel}}</strong> (<a href="{{.UserURL}}">page</a>) uses
  {{if .UseFound}}<a href="{{.UseURL}}">{{.UseLabel}}</a>{{else}}{{.UseLabel}}{{end}}
  at <code>{{.OldVersion}}</code> &mdash; current is <code>{{.CurrentVersion}}</code>.</p>
  {{if .Changelog}}<div class="changed-since"><em>What's changed since (current changelog entries):</em>
  {{range .Changelog}}<div class="changelog-entry"><div class="changelog-audience">{{.Audiences}}</div>{{.HTML}}</div>{{end}}
  </div>{{end}}
</div>
{{end}}
{{else}}<p><em>No breaking outdated uses.</em></p>{{end}}
</section>

<section class="minor">
<h2>Informational (minor)</h2>
{{if .Minor}}
{{range .Minor}}<div class="issue">
  <p><span class="badge badge-minor">minor</span>
  <strong>{{.UserLabel}}</strong> (<a href="{{.UserURL}}">page</a>) uses
  {{if .UseFound}}<a href="{{.UseURL}}">{{.UseLabel}}</a>{{else}}{{.UseLabel}}{{end}}
  at <code>{{.OldVersion}}</code> &mdash; current is <code>{{.CurrentVersion}}</code>.</p>
  {{if .Changelog}}<div class="changed-since"><em>What's changed since (current changelog entries):</em>
  {{range .Changelog}}<div class="changelog-entry"><div class="changelog-audience">{{.Audiences}}</div>{{.HTML}}</div>{{end}}
  </div>{{end}}
</div>
{{end}}
{{else}}<p><em>No informational outdated uses.</em></p>{{end}}
</section>
`))

var indexTmpl = template.Must(template.New("index").Parse(`
<h1>docsweb</h1>
<p><a href="{{.OutdatedLink}}">View outdated uses &rarr;</a></p>
{{range .Groups}}<section>
<h2>{{.Scope}}</h2>
<ul class="plain">
{{range .Targets}}<li><a href="{{.URL}}">{{.Label}}</a></li>
{{end}}</ul>
</section>
{{end}}
`))

// indexPageData additionally needs an OutdatedLink for the inline "view
// outdated uses" link inside the body (the shell's nav bar already links
// there too, but a prominent in-body link is friendlier for a POC).

var changelogTmpl = template.Must(template.New("changelog").Parse(`
<style>
  .cl-filters {
    display: flex;
    flex-wrap: wrap;
    gap: 1rem;
    align-items: flex-end;
    margin-bottom: 1.5rem;
    padding-bottom: 1rem;
    border-bottom: 1px solid #8886;
  }
  .cl-filters label {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    font-size: 0.85rem;
    color: #777;
  }
  .cl-filters input, .cl-filters select {
    font: inherit;
    padding: 0.3rem 0.4rem;
  }
  .cl-filters button {
    font: inherit;
    padding: 0.4rem 0.8rem;
    cursor: pointer;
    align-self: flex-end;
  }
  .cl-status {
    color: #777;
    font-size: 0.9rem;
    min-height: 1.2rem;
    margin-bottom: 0.5rem;
  }
  .cl-day {
    margin-bottom: 1.75rem;
  }
  .cl-day h2 {
    margin-top: 0;
    margin-bottom: 0.75rem;
    font-size: 1.1rem;
    border-bottom: 1px solid #8886;
    padding-bottom: 0.3rem;
  }
  .cl-entry {
    border: 1px solid #8886;
    border-radius: 0.4rem;
    padding: 0.75rem 1rem;
    margin-bottom: 0.75rem;
  }
  .cl-entry-head {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    margin-bottom: 0.4rem;
  }
  .cl-entry-body {
    font-size: 0.95rem;
  }
  .cl-entry-body .changelog-entry {
    margin: 0.4rem 0;
  }
  .badge-patch { background: #dcfce7; color: #166534; }
  .cl-doc-link {
    display: inline-block;
    margin-top: 0.4rem;
    font-size: 0.85rem;
  }
  #cl-load-more {
    display: block;
    margin: 1.5rem auto 0;
    font: inherit;
    padding: 0.5rem 1.2rem;
    cursor: pointer;
  }
</style>

<h1>Changelog</h1>
<p>Every version introduced across every target, newest first. Data is loaded lazily from the
JSON data API as needed - only the day ranges and target versions actually shown are fetched.</p>

<div class="cl-filters">
  <label>From
    <input type="date" id="cl-from">
  </label>
  <label>Until
    <input type="date" id="cl-until">
  </label>
  <label>Change level
    <select id="cl-level">
      <option value="patch" selected>All changes (patch+)</option>
      <option value="minor">Minor and above</option>
      <option value="major">Major only</option>
    </select>
  </label>
  <label>Scope
    <input type="text" id="cl-scope" placeholder="e.g. libs.util">
  </label>
  <button id="cl-reset" type="button">Reset filters</button>
</div>

<div id="cl-status" class="cl-status"></div>
<div id="cl-days"></div>
<button id="cl-load-more" type="button" style="display:none">Load more</button>

<script src="bundle.js"></script>
`))

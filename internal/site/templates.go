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

<script>
(function () {
  'use strict';

  var MIN_CHANGES = 100;
  var LEVEL_RANK = { major: 3, minor: 2, patch: 1 };

  var state = {
    index: {},
    flatDays: [],
    cursor: 0,
    loadedDays: [],
    shardCache: new Map(),
    targetCache: new Map()
  };

  var els = {
    from: document.getElementById('cl-from'),
    until: document.getElementById('cl-until'),
    level: document.getElementById('cl-level'),
    scope: document.getElementById('cl-scope'),
    reset: document.getElementById('cl-reset'),
    status: document.getElementById('cl-status'),
    days: document.getElementById('cl-days'),
    loadMore: document.getElementById('cl-load-more')
  };

  function pad2(n) {
    n = String(n);
    return n.length < 2 ? '0' + n : n;
  }

  function dayKey(y, m, d) {
    return y + '-' + pad2(m) + '-' + pad2(d);
  }

  function shardName(y, m) {
    return y + '-' + pad2(m);
  }

  function setStatus(msg) {
    els.status.textContent = msg || '';
  }

  // loadJSON fetches url's data via a plain, executable <script src> tag
  // rather than fetch()/XHR: opening a generated site straight off disk
  // (file://, no HTTP server) is a normal way to browse it, and browsers
  // block fetch/XHR entirely under file:// ("CORS request not http"),
  // while a <script src> element is not subject to that restriction - the
  // same loophole classic JSONP relied on before CORS existed. Every JSON
  // file this page needs (see internal/site/json.go's writeJSONPFile) has
  // an executable "<url>.js" sibling that calls the global docsweb_jsonp
  // callback below with its own url and data, so this works identically
  // whether the site is opened via file:// or a real HTTP server.
  var jsonpPending = new Map();

  window.docsweb_jsonp = function (url, data) {
    var entry = jsonpPending.get(url);
    if (entry) {
      jsonpPending.delete(url);
      entry.resolve(data);
    }
  };

  function loadJSON(url) {
    var existing = jsonpPending.get(url);
    if (existing) {
      return existing.promise;
    }
    var entry = {};
    entry.promise = new Promise(function (resolve, reject) {
      entry.resolve = resolve;
      var script = document.createElement('script');
      script.src = url + '.js';
      script.async = true;
      script.addEventListener('error', function () {
        jsonpPending.delete(url);
        script.remove();
        reject(new Error('failed to load ' + url));
      });
      script.addEventListener('load', function () {
        script.remove();
      });
      document.head.appendChild(script);
    });
    jsonpPending.set(url, entry);
    return entry.promise;
  }

  function levelPasses(kind, minLevel) {
    return LEVEL_RANK[kind] >= LEVEL_RANK[minLevel];
  }

  function passesFilters(entry) {
    if (!levelPasses(entry.kind, els.level.value)) {
      return false;
    }
    var scope = els.scope.value.trim().toLowerCase();
    if (scope && (entry.scope || '').toLowerCase().indexOf(scope) === -1) {
      return false;
    }
    return true;
  }

  function rebuildFlatDays() {
    var from = els.from.value || null;
    var until = els.until.value || null;
    var days = [];
    Object.keys(state.index).forEach(function (y) {
      Object.keys(state.index[y]).forEach(function (m) {
        state.index[y][m].forEach(function (d) {
          var key = dayKey(y, m, d);
          if (from && key < from) return;
          if (until && key > until) return;
          days.push({ y: Number(y), m: Number(m), d: Number(d), key: key });
        });
      });
    });
    days.sort(function (a, b) { return b.key.localeCompare(a.key); });
    state.flatDays = days;
  }

  function fetchShard(y, m) {
    var key = shardName(y, m);
    if (!state.shardCache.has(key)) {
      var p = loadJSON('changelog/' + key + '.json').catch(function () {
        return { days: [] };
      });
      state.shardCache.set(key, p);
    }
    return state.shardCache.get(key);
  }

  function countVisible() {
    var n = 0;
    state.loadedDays.forEach(function (day) {
      day.versions.forEach(function (v) {
        if (passesFilters(v)) {
          n++;
        }
      });
    });
    return n;
  }

  function loadNextDay() {
    if (state.cursor >= state.flatDays.length) {
      return Promise.resolve(false);
    }
    var day = state.flatDays[state.cursor++];
    return fetchShard(day.y, day.m).then(function (shard) {
      var found = null;
      (shard.days || []).forEach(function (d) {
        if (d.date === day.key) {
          found = d;
        }
      });
      state.loadedDays.push({ date: day.key, versions: found ? found.versions : [] });
      return true;
    });
  }

  function loadUntilThreshold(addAtLeast) {
    setStatus('Loading changelog…');
    var target = countVisible() + addAtLeast;
    function step() {
      if (countVisible() >= target) {
        return Promise.resolve();
      }
      return loadNextDay().then(function (more) {
        if (!more) {
          return;
        }
        return step();
      });
    }
    return step().then(function () {
      render();
      setStatus('');
    });
  }

  function maybeAutoLoadMore() {
    if (countVisible() < MIN_CHANGES && state.cursor < state.flatDays.length) {
      loadUntilThreshold(MIN_CHANGES);
    }
  }

  function onFilterChange() {
    render();
    maybeAutoLoadMore();
  }

  function onDateFilterChange() {
    state.loadedDays = [];
    state.cursor = 0;
    rebuildFlatDays();
    loadUntilThreshold(MIN_CHANGES);
  }

  function onReset() {
    els.from.value = '';
    els.until.value = '';
    els.level.value = 'patch';
    els.scope.value = '';
    onDateFilterChange();
  }

  function debounce(fn, ms) {
    var t = null;
    return function () {
      var args = arguments;
      clearTimeout(t);
      t = setTimeout(function () { fn.apply(null, args); }, ms);
    };
  }

  function targetDir(scope, name) {
    return (scope ? scope.split('.').join('/') + '/' : '') + name;
  }

  // versionJSONURL computes an exact version's own JSON file path directly
  // from scope+name+version - every version, current or historic alike,
  // has one at this address (see internal/site/json.go's writeTargetJSON:
  // the current version's own data is written a second time at this same
  // address scheme), so no separate lookup is ever needed just to find it.
  function versionJSONURL(scope, name, version) {
    return targetDir(scope, name) + '/' + version + '.json';
  }

  // fetchTargetVersion loads one changelog entry's own version JSON,
  // cached by its computed URL since the same target can appear more than
  // once across the loaded date range. Alongside the raw data, it resolves
  // pageURL - the HTML page this content actually belongs to: the
  // version-specific page for a historic version, but the bare canonical
  // page for the current one (data.historic tells them apart, since the
  // URL just fetched is the version-specific one either way) - both the
  // "View full version" link and rewriteEmbeddedLinks (for any relative
  // @link/@uses href inside the fetched changelog HTML) need this exact
  // page, not the JSON path that was actually fetched.
  function fetchTargetVersion(v) {
    var url = versionJSONURL(v.scope, v.name, v.version);
    if (!state.targetCache.has(url)) {
      state.targetCache.set(url, loadJSON(url).then(function (data) {
        var pageURL = data.historic ? url.replace(/\.json$/, '.html') : targetDir(v.scope, v.name) + '.html';
        return { data: data, pageURL: pageURL };
      }));
    }
    return state.targetCache.get(url);
  }

  // rewriteEmbeddedLinks fixes up <a href> targets inside pre-rendered
  // changelog HTML fetched from ownPageURL (a target version's own page,
  // e.g. "docsweb/build.html" or "docsweb/build/v0.1.0.html"). That HTML's
  // relative links (from @link:/@uses cross-references) were computed
  // relative to that page's own location and depth - wrong once inserted
  // into the changelog page, which always lives at the site root. Each
  // relative href is re-resolved against ownPageURL and rewritten to a
  // root-relative path, which then works from any page on the site.
  function rewriteEmbeddedLinks(container, ownPageURL) {
    var base = new URL(ownPageURL, document.baseURI);
    var anchors = container.querySelectorAll('a[href]');
    for (var i = 0; i < anchors.length; i++) {
      var raw = anchors[i].getAttribute('href');
      if (!raw || raw.charAt(0) === '#' || /^[a-z][a-z0-9+.-]*:/i.test(raw)) {
        continue;
      }
      var resolved = new URL(raw, base);
      anchors[i].setAttribute('href', resolved.pathname + resolved.search + resolved.hash);
    }
  }

  function renderEntry(v) {
    var row = document.createElement('div');
    row.className = 'cl-entry';

    var head = document.createElement('div');
    head.className = 'cl-entry-head';
    var badge = document.createElement('span');
    badge.className = 'badge badge-' + v.kind;
    badge.textContent = v.kind;
    head.appendChild(badge);
    var label = document.createElement('strong');
    label.textContent = (v.scope ? v.scope + '.' : '') + v.name + '@' + v.version;
    head.appendChild(label);
    row.appendChild(head);

    var body = document.createElement('div');
    body.className = 'cl-entry-body';
    body.textContent = 'Loading…';
    row.appendChild(body);

    fetchTargetVersion(v).then(function (result) {
      body.textContent = '';
      var data = result.data;
      if (data.changelog && data.changelog.length) {
        data.changelog.forEach(function (c) {
          var entry = document.createElement('div');
          entry.className = 'changelog-entry';
          if (c.audiences && c.audiences.length) {
            var aud = document.createElement('div');
            aud.className = 'changelog-audience';
            aud.textContent = c.audiences.join(', ');
            entry.appendChild(aud);
          }
          var html = document.createElement('div');
          html.innerHTML = c.html;
          rewriteEmbeddedLinks(html, result.pageURL);
          entry.appendChild(html);
          body.appendChild(entry);
        });
      } else {
        body.textContent = 'No changelog text for this version.';
      }
      var link = document.createElement('a');
      link.href = result.pageURL;
      link.className = 'cl-doc-link';
      link.textContent = 'View full version →';
      body.appendChild(link);
    }).catch(function (err) {
      body.textContent = 'Failed to load changelog: ' + err.message;
    });

    return row;
  }

  function renderDay(date, versions) {
    var section = document.createElement('section');
    section.className = 'cl-day';
    var h2 = document.createElement('h2');
    h2.textContent = date;
    section.appendChild(h2);
    versions.forEach(function (v) {
      section.appendChild(renderEntry(v));
    });
    return section;
  }

  function render() {
    els.days.innerHTML = '';
    var shown = 0;
    state.loadedDays.forEach(function (day) {
      var visible = day.versions.filter(passesFilters).slice().sort(function (a, b) {
        var an = (a.scope ? a.scope + '.' : '') + a.name;
        var bn = (b.scope ? b.scope + '.' : '') + b.name;
        return an.localeCompare(bn);
      });
      if (visible.length === 0) {
        return;
      }
      shown += visible.length;
      els.days.appendChild(renderDay(day.date, visible));
    });
    els.loadMore.style.display = state.cursor < state.flatDays.length ? '' : 'none';
    if (shown === 0) {
      var p = document.createElement('p');
      p.textContent = 'No changes match the current filters.';
      els.days.appendChild(p);
    }
  }

  els.level.addEventListener('change', onFilterChange);
  els.scope.addEventListener('input', debounce(onFilterChange, 200));
  els.from.addEventListener('change', onDateFilterChange);
  els.until.addEventListener('change', onDateFilterChange);
  els.reset.addEventListener('click', onReset);
  els.loadMore.addEventListener('click', function () { loadUntilThreshold(MIN_CHANGES); });

  setStatus('Loading changelog index…');
  loadJSON('changelog/index.json').catch(function () {
    return {};
  }).then(function (index) {
    state.index = index || {};
    rebuildFlatDays();
    return loadUntilThreshold(MIN_CHANGES);
  });
})();
</script>
`))

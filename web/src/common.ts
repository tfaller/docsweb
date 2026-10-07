// Helpers shared by the generated site's client-side scripts (the Changelog
// tab and the outdated-uses page's "what's changed since" blocks) - see
// internal/site/site.go's package doc for the JSON data API they load from.

/** One target's @changelog entry, already rendered - json.go's ChangelogEntry. */
export interface ChangelogEntryJSON {
  audiences?: string[];
  body: string;
  html: string;
}

/** The fields of json.go's Target the client scripts actually read. */
export interface TargetVersionJSON {
  historic: boolean;
  changelog?: ChangelogEntryJSON[];
}

/** One entry of a target's versions.json - json.go's VersionLink. */
export interface VersionLinkJSON {
  version: string;
  current: boolean;
}

/** fetchTargetVersion's resolved result: raw JSON plus the HTML page it belongs to. */
export interface FetchedTargetVersion {
  data: TargetVersionJSON;
  pageURL: string;
}

declare global {
  interface Window {
    docsweb_jsonp: (url: string, data: unknown) => void;
  }
}

// loadJSON fetches url's data via a plain, executable <script src> tag
// rather than fetch()/XHR: opening a generated site straight off disk
// (file://, no HTTP server) is a normal way to browse it, and browsers
// block fetch/XHR entirely under file:// ("CORS request not http"),
// while a <script src> element is not subject to that restriction - the
// same loophole classic JSONP relied on before CORS existed. Every JSON
// file the site's pages need (see internal/site/json.go's writeJSONPFile)
// has an executable "<url>.js" sibling that calls the global docsweb_jsonp
// callback below with its own url and data, so this works identically
// whether the site is opened via file:// or a real HTTP server.
interface PendingLoad {
  promise: Promise<unknown>;
  resolve: (data: unknown) => void;
}
const jsonpPending = new Map<string, PendingLoad>();

window.docsweb_jsonp = function (url: string, data: unknown): void {
  const entry = jsonpPending.get(url);
  if (entry) {
    jsonpPending.delete(url);
    entry.resolve(data);
  }
};

export function loadJSON<T>(url: string): Promise<T> {
  const existing = jsonpPending.get(url);
  if (existing) {
    return existing.promise as Promise<T>;
  }
  const entry = {} as PendingLoad;
  entry.promise = new Promise(function (resolve, reject) {
    entry.resolve = resolve;
    const script = document.createElement("script");
    script.src = url + ".js";
    script.async = true;
    script.addEventListener("error", function () {
      jsonpPending.delete(url);
      script.remove();
      reject(new Error("failed to load " + url));
    });
    script.addEventListener("load", function () {
      script.remove();
    });
    document.head.appendChild(script);
  });
  jsonpPending.set(url, entry);
  return entry.promise as Promise<T>;
}

export function targetDir(scope: string, name: string): string {
  return (scope ? scope.split(".").join("/") + "/" : "") + name;
}

// versionJSONURL computes an exact version's own JSON file path directly
// from scope+name+version - every version, current or historic alike,
// has one at this address (see internal/site/json.go's writeTargetJSON:
// the current version's own data is written a second time at this same
// address scheme), so no separate lookup is ever needed just to find it.
export function versionJSONURL(scope: string, name: string, version: string): string {
  return targetDir(scope, name) + "/" + version + ".json";
}

/** versionsJSONURL is the path of a target's versions.json (every known version). */
export function versionsJSONURL(scope: string, name: string): string {
  return targetDir(scope, name) + "/versions.json";
}

const targetCache = new Map<string, Promise<FetchedTargetVersion>>();

// fetchTargetVersion loads one version's own JSON, cached by its computed
// URL since the same target version can be needed more than once on a
// page. Alongside the raw data, it resolves pageURL - the HTML page this
// content actually belongs to: the version-specific page for a historic
// version, but the bare canonical page for the current one (data.historic
// tells them apart, since the URL just fetched is the version-specific one
// either way) - both "View full version" links and rewriteEmbeddedLinks
// (for any relative @link/@uses href inside the fetched changelog HTML)
// need this exact page, not the JSON path that was actually fetched.
export function fetchTargetVersion(scope: string, name: string, version: string): Promise<FetchedTargetVersion> {
  const url = versionJSONURL(scope, name, version);
  let p = targetCache.get(url);
  if (!p) {
    p = loadJSON<TargetVersionJSON>(url).then(function (data) {
      const pageURL = data.historic ? url.replace(/\.json$/, ".html") : targetDir(scope, name) + ".html";
      return { data: data, pageURL: pageURL };
    });
    targetCache.set(url, p);
  }
  return p;
}

// rewriteEmbeddedLinks fixes up <a href> targets inside pre-rendered
// changelog HTML fetched from ownPageURL (a target version's own page,
// e.g. "docsweb/build.html" or "docsweb/build/v0.1.0.html"). That HTML's
// relative links (from @link:/@uses cross-references) were computed
// relative to that page's own location and depth - wrong once inserted
// into a page at the site root. Each relative href is re-resolved against
// ownPageURL and rewritten to a root-relative path, which then works from
// any page on the site.
export function rewriteEmbeddedLinks(container: HTMLElement, ownPageURL: string): void {
  const base = new URL(ownPageURL, document.baseURI);
  const anchors = container.querySelectorAll("a[href]");
  for (let i = 0; i < anchors.length; i++) {
    const raw = anchors[i].getAttribute("href");
    if (!raw || raw.charAt(0) === "#" || /^[a-z][a-z0-9+.-]*:/i.test(raw)) {
      continue;
    }
    const resolved = new URL(raw, base);
    anchors[i].setAttribute("href", resolved.pathname + resolved.search + resolved.hash);
  }
}

// appendChangelogEntries renders a fetched version's @changelog entries
// into body, or a placeholder if it has none.
export function appendChangelogEntries(body: HTMLElement, result: FetchedTargetVersion): void {
  const changelog = result.data.changelog;
  if (!changelog || !changelog.length) {
    body.appendChild(document.createTextNode("No changelog text for this version."));
    return;
  }
  changelog.forEach(function (c) {
    const entry = document.createElement("div");
    entry.className = "changelog-entry";
    if (c.audiences && c.audiences.length) {
      const aud = document.createElement("div");
      aud.className = "changelog-audience";
      aud.textContent = c.audiences.join(", ");
      entry.appendChild(aud);
    }
    const html = document.createElement("div");
    html.innerHTML = c.html;
    rewriteEmbeddedLinks(html, result.pageURL);
    entry.appendChild(html);
    body.appendChild(entry);
  });
}

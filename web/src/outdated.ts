// Client-side behind the outdated-uses page: fills each ".changed-since"
// placeholder (internal/site/templates.go's outdatedTmpl) with the
// changelog of every version of the referenced target newer than the one
// the @uses reference was written against. Only data for the rows on the
// page is ever fetched: the target's versions.json, then one version JSON
// per newer version - see internal/site/json.go for the data API.

import { appendChangelogEntries, fetchTargetVersion, loadJSON, versionsJSONURL, VersionLinkJSON } from "./common";

/** Parses "vMAJOR.MINOR.PATCH" (a pre-release/build suffix is ignored); null if malformed. */
function parseVersion(v: string): [number, number, number] | null {
  const m = /^v?(\d+)\.(\d+)\.(\d+)/.exec(v);
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
}

function compareVersions(a: [number, number, number], b: [number, number, number]): number {
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) {
      return a[i] - b[i];
    }
  }
  return 0;
}

/** Every known version strictly newer than old, oldest first. */
export function versionsNewerThan(versions: VersionLinkJSON[], old: string): string[] {
  const oldParsed = parseVersion(old);
  if (!oldParsed) {
    return [];
  }
  const newer: { v: string; p: [number, number, number] }[] = [];
  versions.forEach(function (link) {
    const p = parseVersion(link.version);
    if (p && compareVersions(p, oldParsed) > 0) {
      newer.push({ v: link.version, p: p });
    }
  });
  newer.sort(function (a, b) {
    return compareVersions(a.p, b.p);
  });
  return newer.map(function (n) {
    return n.v;
  });
}

function fillRow(container: HTMLElement): void {
  const scope = container.dataset.scope || "";
  const name = container.dataset.name || "";
  const old = container.dataset.old || "";

  const status = container.querySelector(".changed-since-status") as HTMLElement;
  const list = container.querySelector(".changed-since-versions") as HTMLElement;

  loadJSON<VersionLinkJSON[]>(versionsJSONURL(scope, name))
    .then(function (versions) {
      const newer = versionsNewerThan(versions || [], old);
      if (!newer.length) {
        status.textContent = "No newer versions found.";
        return;
      }
      status.textContent = "";
      newer.forEach(function (version) {
        const section = document.createElement("div");
        section.className = "changed-since-version";
        const head = document.createElement("strong");
        head.textContent = version;
        section.appendChild(head);
        const body = document.createElement("div");
        body.textContent = "Loading…";
        section.appendChild(body);
        list.appendChild(section);

        fetchTargetVersion(scope, name, version)
          .then(function (result) {
            body.textContent = "";
            appendChangelogEntries(body, result);
            const link = document.createElement("a");
            link.href = result.pageURL;
            link.className = "cl-doc-link";
            link.textContent = "View full version →";
            body.appendChild(link);
          })
          .catch(function (err: Error) {
            body.textContent = "Failed to load changelog: " + err.message;
          });
      });
    })
    .catch(function (err: Error) {
      status.textContent = "Failed to load version list: " + err.message;
    });
}

export function initOutdated(): void {
  const rows = document.querySelectorAll<HTMLElement>(".changed-since[data-name]");
  for (let i = 0; i < rows.length; i++) {
    fillRow(rows[i]);
  }
}

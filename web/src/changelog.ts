// Client-side app behind the generated site's "Changelog" tab
// (internal/site/templates.go's changelogTmpl writes only the static shell;
// this script does everything else). See internal/site/site.go's package
// doc for the JSON data API this loads from.

import { appendChangelogEntries, fetchTargetVersion, loadJSON } from "./common";

/** One target version introduced on a given day - internal/site/json.go's ChangelogVersion. */
interface ChangelogVersionEntry {
  scope: string;
  name: string;
  version: string;
  commitHash?: string;
  kind: "major" | "minor" | "patch";
}

/** Every version introduced on one UTC calendar day - json.go's ChangelogDay. */
interface ChangelogDayJSON {
  date: string;
  versions: ChangelogVersionEntry[];
}

/** One changelog/<YYYY-MM>.json file's content - json.go's ChangelogShard. */
interface ChangelogShard {
  days?: ChangelogDayJSON[];
}

/** changelog/index.json's content - json.go's ChangelogIndex: {year: {month: [day, ...]}}. */
type ChangelogIndex = Record<string, Record<string, number[]>>;

/** A day already loaded (fetched + resolved to its shard's entry, if any). */
interface LoadedDay {
  date: string;
  versions: ChangelogVersionEntry[];
}

/** One calendar day queued for loading, before its shard has been fetched. */
interface FlatDay {
  y: number;
  m: number;
  d: number;
  key: string;
}

export function initChangelog(): void {
  // Only the Changelog tab's own page has this shell; any other page
  // loading the shared bundle has nothing to do here.
  if (!document.getElementById("cl-days")) {
    return;
  }

  const MIN_CHANGES = 100;
  const LEVEL_RANK: Record<string, number> = { major: 3, minor: 2, patch: 1 };

  const state = {
    index: {} as ChangelogIndex,
    flatDays: [] as FlatDay[],
    cursor: 0,
    loadedDays: [] as LoadedDay[],
    shardCache: new Map<string, Promise<ChangelogShard>>(),
  };

  const els = {
    from: document.getElementById("cl-from") as HTMLInputElement,
    until: document.getElementById("cl-until") as HTMLInputElement,
    level: document.getElementById("cl-level") as HTMLSelectElement,
    scope: document.getElementById("cl-scope") as HTMLInputElement,
    reset: document.getElementById("cl-reset") as HTMLButtonElement,
    status: document.getElementById("cl-status") as HTMLElement,
    days: document.getElementById("cl-days") as HTMLElement,
    loadMore: document.getElementById("cl-load-more") as HTMLButtonElement,
  };

  function pad2(n: number): string {
    const s = String(n);
    return s.length < 2 ? "0" + s : s;
  }

  function dayKey(y: number, m: number, d: number): string {
    return y + "-" + pad2(m) + "-" + pad2(d);
  }

  function shardName(y: number, m: number): string {
    return y + "-" + pad2(m);
  }

  function setStatus(msg: string): void {
    els.status.textContent = msg || "";
  }

  function levelPasses(kind: string, minLevel: string): boolean {
    return LEVEL_RANK[kind] >= LEVEL_RANK[minLevel];
  }

  function passesFilters(entry: ChangelogVersionEntry): boolean {
    if (!levelPasses(entry.kind, els.level.value)) {
      return false;
    }
    const scope = els.scope.value.trim().toLowerCase();
    if (scope && (entry.scope || "").toLowerCase().indexOf(scope) === -1) {
      return false;
    }
    return true;
  }

  function rebuildFlatDays(): void {
    const from = els.from.value || null;
    const until = els.until.value || null;
    const days: FlatDay[] = [];
    Object.keys(state.index).forEach(function (y) {
      Object.keys(state.index[y]).forEach(function (m) {
        state.index[y][m].forEach(function (d) {
          const key = dayKey(Number(y), Number(m), d);
          if (from && key < from) return;
          if (until && key > until) return;
          days.push({ y: Number(y), m: Number(m), d: d, key: key });
        });
      });
    });
    days.sort(function (a, b) {
      return b.key.localeCompare(a.key);
    });
    state.flatDays = days;
  }

  function fetchShard(y: number, m: number): Promise<ChangelogShard> {
    const key = shardName(y, m);
    if (!state.shardCache.has(key)) {
      const p = loadJSON<ChangelogShard>("changelog/" + key + ".json").catch(function () {
        return { days: [] };
      });
      state.shardCache.set(key, p);
    }
    return state.shardCache.get(key)!;
  }

  function countVisible(): number {
    let n = 0;
    state.loadedDays.forEach(function (day) {
      day.versions.forEach(function (v) {
        if (passesFilters(v)) {
          n++;
        }
      });
    });
    return n;
  }

  function loadNextDay(): Promise<boolean> {
    if (state.cursor >= state.flatDays.length) {
      return Promise.resolve(false);
    }
    const day = state.flatDays[state.cursor++];
    return fetchShard(day.y, day.m).then(function (shard) {
      let found: ChangelogDayJSON | null = null;
      (shard.days || []).forEach(function (d) {
        if (d.date === day.key) {
          found = d;
        }
      });
      state.loadedDays.push({ date: day.key, versions: found ? (found as ChangelogDayJSON).versions : [] });
      return true;
    });
  }

  function loadUntilThreshold(addAtLeast: number): Promise<void> {
    setStatus("Loading changelog…");
    const target = countVisible() + addAtLeast;
    function step(): Promise<void> {
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
      setStatus("");
    });
  }

  function maybeAutoLoadMore(): void {
    if (countVisible() < MIN_CHANGES && state.cursor < state.flatDays.length) {
      loadUntilThreshold(MIN_CHANGES);
    }
  }

  function onFilterChange(): void {
    render();
    maybeAutoLoadMore();
  }

  function onDateFilterChange(): void {
    state.loadedDays = [];
    state.cursor = 0;
    rebuildFlatDays();
    loadUntilThreshold(MIN_CHANGES);
  }

  function onReset(): void {
    els.from.value = "";
    els.until.value = "";
    els.level.value = "patch";
    els.scope.value = "";
    onDateFilterChange();
  }

  function debounce<A extends unknown[]>(fn: (...args: A) => void, ms: number): (...args: A) => void {
    let t: ReturnType<typeof setTimeout> | null = null;
    return function (...args: A) {
      if (t !== null) {
        clearTimeout(t);
      }
      t = setTimeout(function () {
        fn.apply(null, args);
      }, ms);
    };
  }

  function renderEntry(v: ChangelogVersionEntry): HTMLElement {
    const row = document.createElement("div");
    row.className = "cl-entry";

    const head = document.createElement("div");
    head.className = "cl-entry-head";
    const badge = document.createElement("span");
    badge.className = "badge badge-" + v.kind;
    badge.textContent = v.kind;
    head.appendChild(badge);
    const label = document.createElement("strong");
    label.textContent = (v.scope ? v.scope + "." : "") + v.name + "@" + v.version;
    head.appendChild(label);
    row.appendChild(head);

    const body = document.createElement("div");
    body.className = "cl-entry-body";
    body.textContent = "Loading…";
    row.appendChild(body);

    fetchTargetVersion(v.scope, v.name, v.version)
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

    return row;
  }

  function renderDay(date: string, versions: ChangelogVersionEntry[]): HTMLElement {
    const section = document.createElement("section");
    section.className = "cl-day";
    const h2 = document.createElement("h2");
    h2.textContent = date;
    section.appendChild(h2);
    versions.forEach(function (v) {
      section.appendChild(renderEntry(v));
    });
    return section;
  }

  function render(): void {
    els.days.innerHTML = "";
    let shown = 0;
    state.loadedDays.forEach(function (day) {
      const visible = day.versions
        .filter(passesFilters)
        .slice()
        .sort(function (a, b) {
          const an = (a.scope ? a.scope + "." : "") + a.name;
          const bn = (b.scope ? b.scope + "." : "") + b.name;
          return an.localeCompare(bn);
        });
      if (visible.length === 0) {
        return;
      }
      shown += visible.length;
      els.days.appendChild(renderDay(day.date, visible));
    });
    els.loadMore.style.display = state.cursor < state.flatDays.length ? "" : "none";
    if (shown === 0) {
      const p = document.createElement("p");
      p.textContent = "No changes match the current filters.";
      els.days.appendChild(p);
    }
  }

  els.level.addEventListener("change", onFilterChange);
  els.scope.addEventListener("input", debounce(onFilterChange, 200));
  els.from.addEventListener("change", onDateFilterChange);
  els.until.addEventListener("change", onDateFilterChange);
  els.reset.addEventListener("click", onReset);
  els.loadMore.addEventListener("click", function () {
    loadUntilThreshold(MIN_CHANGES);
  });

  setStatus("Loading changelog index…");
  loadJSON<ChangelogIndex>("changelog/index.json")
    .catch(function () {
      return {} as ChangelogIndex;
    })
    .then(function (index) {
      state.index = index || {};
      rebuildFlatDays();
      return loadUntilThreshold(MIN_CHANGES);
    });
}

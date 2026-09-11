(function() {
	//#region src/changelog.ts
	(function() {
		"use strict";
		const MIN_CHANGES = 100;
		const LEVEL_RANK = {
			major: 3,
			minor: 2,
			patch: 1
		};
		const state = {
			index: {},
			flatDays: [],
			cursor: 0,
			loadedDays: [],
			shardCache: /* @__PURE__ */ new Map(),
			targetCache: /* @__PURE__ */ new Map()
		};
		const els = {
			from: document.getElementById("cl-from"),
			until: document.getElementById("cl-until"),
			level: document.getElementById("cl-level"),
			scope: document.getElementById("cl-scope"),
			reset: document.getElementById("cl-reset"),
			status: document.getElementById("cl-status"),
			days: document.getElementById("cl-days"),
			loadMore: document.getElementById("cl-load-more")
		};
		function pad2(n) {
			const s = String(n);
			return s.length < 2 ? "0" + s : s;
		}
		function dayKey(y, m, d) {
			return y + "-" + pad2(m) + "-" + pad2(d);
		}
		function shardName(y, m) {
			return y + "-" + pad2(m);
		}
		function setStatus(msg) {
			els.status.textContent = msg || "";
		}
		const jsonpPending = /* @__PURE__ */ new Map();
		window.docsweb_jsonp = function(url, data) {
			const entry = jsonpPending.get(url);
			if (entry) {
				jsonpPending.delete(url);
				entry.resolve(data);
			}
		};
		function loadJSON(url) {
			const existing = jsonpPending.get(url);
			if (existing) return existing.promise;
			const entry = {};
			entry.promise = new Promise(function(resolve, reject) {
				entry.resolve = resolve;
				const script = document.createElement("script");
				script.src = url + ".js";
				script.async = true;
				script.addEventListener("error", function() {
					jsonpPending.delete(url);
					script.remove();
					reject(/* @__PURE__ */ new Error("failed to load " + url));
				});
				script.addEventListener("load", function() {
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
			if (!levelPasses(entry.kind, els.level.value)) return false;
			const scope = els.scope.value.trim().toLowerCase();
			if (scope && (entry.scope || "").toLowerCase().indexOf(scope) === -1) return false;
			return true;
		}
		function rebuildFlatDays() {
			const from = els.from.value || null;
			const until = els.until.value || null;
			const days = [];
			Object.keys(state.index).forEach(function(y) {
				Object.keys(state.index[y]).forEach(function(m) {
					state.index[y][m].forEach(function(d) {
						const key = dayKey(Number(y), Number(m), d);
						if (from && key < from) return;
						if (until && key > until) return;
						days.push({
							y: Number(y),
							m: Number(m),
							d,
							key
						});
					});
				});
			});
			days.sort(function(a, b) {
				return b.key.localeCompare(a.key);
			});
			state.flatDays = days;
		}
		function fetchShard(y, m) {
			const key = shardName(y, m);
			if (!state.shardCache.has(key)) {
				const p = loadJSON("changelog/" + key + ".json").catch(function() {
					return { days: [] };
				});
				state.shardCache.set(key, p);
			}
			return state.shardCache.get(key);
		}
		function countVisible() {
			let n = 0;
			state.loadedDays.forEach(function(day) {
				day.versions.forEach(function(v) {
					if (passesFilters(v)) n++;
				});
			});
			return n;
		}
		function loadNextDay() {
			if (state.cursor >= state.flatDays.length) return Promise.resolve(false);
			const day = state.flatDays[state.cursor++];
			return fetchShard(day.y, day.m).then(function(shard) {
				let found = null;
				(shard.days || []).forEach(function(d) {
					if (d.date === day.key) found = d;
				});
				state.loadedDays.push({
					date: day.key,
					versions: found ? found.versions : []
				});
				return true;
			});
		}
		function loadUntilThreshold(addAtLeast) {
			setStatus("Loading changelog…");
			const target = countVisible() + addAtLeast;
			function step() {
				if (countVisible() >= target) return Promise.resolve();
				return loadNextDay().then(function(more) {
					if (!more) return;
					return step();
				});
			}
			return step().then(function() {
				render();
				setStatus("");
			});
		}
		function maybeAutoLoadMore() {
			if (countVisible() < MIN_CHANGES && state.cursor < state.flatDays.length) loadUntilThreshold(MIN_CHANGES);
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
			els.from.value = "";
			els.until.value = "";
			els.level.value = "patch";
			els.scope.value = "";
			onDateFilterChange();
		}
		function debounce(fn, ms) {
			let t = null;
			return function(...args) {
				if (t !== null) clearTimeout(t);
				t = setTimeout(function() {
					fn.apply(null, args);
				}, ms);
			};
		}
		function targetDir(scope, name) {
			return (scope ? scope.split(".").join("/") + "/" : "") + name;
		}
		function versionJSONURL(scope, name, version) {
			return targetDir(scope, name) + "/" + version + ".json";
		}
		function fetchTargetVersion(v) {
			const url = versionJSONURL(v.scope, v.name, v.version);
			if (!state.targetCache.has(url)) state.targetCache.set(url, loadJSON(url).then(function(data) {
				return {
					data,
					pageURL: data.historic ? url.replace(/\.json$/, ".html") : targetDir(v.scope, v.name) + ".html"
				};
			}));
			return state.targetCache.get(url);
		}
		function rewriteEmbeddedLinks(container, ownPageURL) {
			const base = new URL(ownPageURL, document.baseURI);
			const anchors = container.querySelectorAll("a[href]");
			for (let i = 0; i < anchors.length; i++) {
				const raw = anchors[i].getAttribute("href");
				if (!raw || raw.charAt(0) === "#" || /^[a-z][a-z0-9+.-]*:/i.test(raw)) continue;
				const resolved = new URL(raw, base);
				anchors[i].setAttribute("href", resolved.pathname + resolved.search + resolved.hash);
			}
		}
		function renderEntry(v) {
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
			fetchTargetVersion(v).then(function(result) {
				body.textContent = "";
				const data = result.data;
				if (data.changelog && data.changelog.length) data.changelog.forEach(function(c) {
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
				else body.textContent = "No changelog text for this version.";
				const link = document.createElement("a");
				link.href = result.pageURL;
				link.className = "cl-doc-link";
				link.textContent = "View full version →";
				body.appendChild(link);
			}).catch(function(err) {
				body.textContent = "Failed to load changelog: " + err.message;
			});
			return row;
		}
		function renderDay(date, versions) {
			const section = document.createElement("section");
			section.className = "cl-day";
			const h2 = document.createElement("h2");
			h2.textContent = date;
			section.appendChild(h2);
			versions.forEach(function(v) {
				section.appendChild(renderEntry(v));
			});
			return section;
		}
		function render() {
			els.days.innerHTML = "";
			let shown = 0;
			state.loadedDays.forEach(function(day) {
				const visible = day.versions.filter(passesFilters).slice().sort(function(a, b) {
					const an = (a.scope ? a.scope + "." : "") + a.name;
					const bn = (b.scope ? b.scope + "." : "") + b.name;
					return an.localeCompare(bn);
				});
				if (visible.length === 0) return;
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
		els.loadMore.addEventListener("click", function() {
			loadUntilThreshold(MIN_CHANGES);
		});
		setStatus("Loading changelog index…");
		loadJSON("changelog/index.json").catch(function() {
			return {};
		}).then(function(index) {
			state.index = index || {};
			rebuildFlatDays();
			return loadUntilThreshold(MIN_CHANGES);
		});
	})();
	//#endregion
})();

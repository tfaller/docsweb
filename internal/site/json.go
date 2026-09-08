package site

// json.go renders the same build.Result as a parallel JSON data API,
// alongside (never instead of) the HTML pages Generate already writes -
// one JSON file per HTML page it mirrors, plus a per-target versions.json
// and a date-indexed changelog feed with no HTML equivalent:
//
//   - <target>.json - mirrors the current target page (build.TargetURL).
//     See Target.
//   - <target>/vX.Y.Z.json - mirrors a past version's page
//     (build.HistoricTargetURL) for every version internal/history
//     discovered, *plus* one more for the current version itself
//     (identical content to <target>.json) - so every version, current or
//     historic alike, is addressable purely from scope+name+version, with
//     no need to first discover whether a given version happens to be the
//     current one. See Target and writeTargetJSON.
//   - <target>/versions.json - every known version of that target (mirrors
//     the version-switcher list every target page shows), for a client
//     that wants to discover/browse them all at once rather than address
//     one directly. See VersionLink.
//   - index.json / _outdated.json - mirror the two site-wide HTML pages.
//     See IndexPage, OutdatedPage.
//   - changelog/<YYYY-MM>.json - every version introduced that month,
//     grouped by day, each carrying its major/minor/patch Kind against the
//     immediately preceding known version. See ChangelogShard.
//   - changelog/index.json - which months/days have a changelog entry at
//     all, as {year: {month: [day, ...]}} with month/day as bare integers
//     (not zero-padded) - both compact and, since integer-like string keys
//     enumerate in ascending numeric order in JSON/JS regardless of
//     insertion order, already in the right order for a calendar UI to
//     walk without re-sorting.
//
// Every exported type here is the decoding target for a JSON file this
// package writes - exported (rather than kept alongside the HTML-only
// unexported template data types) so a caller, or this package's own
// tests, can decode a generated file without redeclaring its shape.
//
// Every cross-reference URL here reuses the relative path already computed
// for the HTML page it mirrors (build.UseLink.URL, the "used by"/version
// links site.go computes) and simply swaps the trailing ".html" for
// ".json" - safe because the JSON tree mirrors the HTML tree file-for-file,
// so a relative path that resolves correctly from an HTML page resolves
// identically from its JSON sibling. A reference to another target (Uses,
// ChangelogVersion) instead gives just its scope/name/version and no
// resolved URL at all: since <target>/vX.Y.Z.json now exists for every
// version (see above), a client can always compute the right file itself -
// there is no current-vs-historic ambiguity left to resolve via a separate
// lookup (versions.json remains useful for browsing every version at once,
// just no longer required to address one specific version).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tfaller/docsweb/internal/build"
	"github.com/tfaller/docsweb/internal/check"
	"github.com/tfaller/docsweb/internal/model"
)

// jsonURL converts a *.html URL - root-relative or already made relative to
// some page via build.RelLink - into its *.json sibling.
func jsonURL(htmlURL string) string {
	return strings.TrimSuffix(htmlURL, ".html") + ".json"
}

// commitTimePtr reports t as an RFC3339 timestamp pointer, or nil for the
// zero time (encoding/json's omitempty does not treat a zero time.Time as
// empty, since it's a struct) - the same best-effort-unknown case
// build.RenderedTarget.CommitTime documents.
func commitTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// ChangelogEntry is one target's @changelog entry, mirroring
// model.ChangelogEntry plus its already-rendered HTML.
type ChangelogEntry struct {
	Audiences []model.Audience `json:"audiences,omitempty"`
	Body      string           `json:"body"`
	HTML      string           `json:"html"`
}

// UseLink is one entry in Target.Uses - see the package doc for why it
// carries no resolved URL.
type UseLink struct {
	// Label is "scope.name@vX.Y.Z", exactly as the @uses reference reads.
	Label string `json:"label"`
	// Found is false only when the referenced target doesn't exist in this
	// build at all - never for a reference to a version internal/history
	// simply never discovered, which falls back to that target's current
	// version instead (see build.resolveUses/lookupVersion).
	Found bool `json:"found"`
}

// UsedByRef is one entry in Target.UsedBy - the reverse of a @uses edge.
// Unlike UseLink/ChangelogVersion, this does carry a resolved URL: it's
// only ever emitted for a current-version page (see writeTargetJSON), so
// there's exactly one URL scheme to apply, not a current-vs-historic
// ambiguity to defer to a lookup.
type UsedByRef struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// VersionLink is one entry in a target's versions.json - see the package
// doc.
type VersionLink struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Current bool   `json:"current"`
	// CommitHash is the full (40-hex-digit) hash of the commit that
	// introduced this version, or "" if unknown (see
	// build.RenderedTarget.CommitHash).
	CommitHash string     `json:"commitHash,omitempty"`
	CommitTime *time.Time `json:"commitTime,omitempty"`
}

// Target is one version of a target - current or historic - as written to
// <target>.json / <target>/vX.Y.Z.json.
type Target struct {
	Scope       string           `json:"scope"`
	Name        string           `json:"name"`
	DisplayName string           `json:"displayName"`
	Version     string           `json:"version"`
	Audiences   []model.Audience `json:"audiences,omitempty"`
	Summary     string           `json:"summary,omitempty"`
	SummaryHTML string           `json:"summaryHtml,omitempty"`
	Doc         string           `json:"doc"`
	DocHTML     string           `json:"docHtml"`
	Changelog   []ChangelogEntry `json:"changelog,omitempty"`
	Uses        []UseLink        `json:"uses,omitempty"`
	UsedBy      []UsedByRef      `json:"usedBy,omitempty"`
	Author      string           `json:"author,omitempty"`
	// CommitHash is the full (40-hex-digit) hash of the commit that
	// introduced this version, or "" if unknown (see
	// build.RenderedTarget.CommitHash).
	CommitHash string     `json:"commitHash,omitempty"`
	CommitTime *time.Time `json:"commitTime,omitempty"`
	// Historic is true for a past-version file (build.RenderedTarget.
	// History) - see targetPageData.IsHistoric.
	Historic bool `json:"historic"`
	// VersionsURL points at this target's versions.json (same for the
	// current file and every one of its historic siblings).
	VersionsURL string `json:"versionsUrl"`
}

// writeTargetJSON writes rt's current-version JSON file, one JSON file per
// rt.History entry, and rt's shared versions.json - the JSON-tree
// equivalent of writeTargetPage.
func writeTargetJSON(outDir string, rt *build.RenderedTarget) error {
	t := rt.Target
	htmlURL := build.TargetURL(t.Ref())
	versionsURL := strings.TrimSuffix(htmlURL, ".html") + "/versions.json"

	data := buildTargetJSON(t, rt.SummaryHTML, rt.DocHTML, rt.ChangelogHTML, rt.Author, rt.CommitHash, rt.CommitTime, rt.Uses, rt.UsedBy, htmlURL, versionsURL, false)
	if err := writeJSONFile(outDir, jsonURL(htmlURL), data); err != nil {
		return err
	}

	// Also write the current version's exact same data a second time, at
	// its own version-specific path - the same address scheme
	// HistoricTargetURL gives a past version. This makes every version's
	// JSON file addressable purely from scope+name+version, with no need
	// to first discover whether that version happens to be the current
	// one - see ChangelogVersion (which relies on exactly this) in the
	// package doc.
	currentVersionedURL := jsonURL(build.HistoricTargetURL(t.Ref()))
	if err := writeJSONFile(outDir, currentVersionedURL, data); err != nil {
		return err
	}

	for _, h := range rt.History {
		hHTMLURL := build.HistoricTargetURL(h.Target.Ref())
		// Historic pages carry no "used by" data of their own - see
		// targetPageData.IsHistoric - so htmlPageURL is unused (no usedBy
		// entries to resolve relative to it).
		hdata := buildTargetJSON(h.Target, h.SummaryHTML, h.DocHTML, h.ChangelogHTML, h.Author, h.CommitHash, h.CommitTime, h.Uses, nil, "", versionsURL, true)
		if err := writeJSONFile(outDir, jsonURL(hHTMLURL), hdata); err != nil {
			return err
		}
	}

	return writeVersionsJSON(outDir, versionsURL, rt.Versions)
}

// buildTargetJSON builds one version's JSON payload - the current one, or
// one historic one - mirroring buildTargetPageData's fields.
func buildTargetJSON(
	t *model.Target,
	summaryHTML, docHTML string,
	changelog []build.ChangelogHTML,
	author, commitHash string,
	commitTime time.Time,
	uses []build.UseLink,
	usedBy []check.UsedByRef,
	htmlPageURL string,
	versionsURL string,
	historic bool,
) Target {
	data := Target{
		Scope:       t.Scope,
		Name:        t.Name,
		DisplayName: displayName(t),
		Version:     t.Version.String(),
		Audiences:   t.Audiences,
		Summary:     t.Summary,
		SummaryHTML: summaryHTML,
		Doc:         t.Doc,
		DocHTML:     docHTML,
		Author:      author,
		CommitHash:  commitHash,
		CommitTime:  commitTimePtr(commitTime),
		Historic:    historic,
		VersionsURL: versionsURL,
	}

	for _, u := range uses {
		data.Uses = append(data.Uses, UseLink{Label: u.Label, Found: u.Found})
	}

	for _, ub := range usedBy {
		label := fmt.Sprintf("%s@%s", ub.User.Key(), ub.User.Version)
		relHTML := build.RelLink(htmlPageURL, build.TargetURL(ub.User))
		data.UsedBy = append(data.UsedBy, UsedByRef{Label: label, URL: jsonURL(relHTML)})
	}

	// changelog is always built by iterating t.Changelog in source order
	// (see build.Run/renderHistoricVersion), so indexing t.Changelog[i]
	// for changelog[i]'s raw Body is safe.
	for i, c := range changelog {
		entry := ChangelogEntry{Audiences: c.Audiences, HTML: c.HTML}
		if i < len(t.Changelog) {
			entry.Body = t.Changelog[i].Body
		}
		data.Changelog = append(data.Changelog, entry)
	}

	return data
}

func writeVersionsJSON(outDir, url string, versions []build.VersionLink) error {
	out := make([]VersionLink, len(versions))
	for i, v := range versions {
		out[i] = VersionLink{
			Version:    v.Version.String(),
			URL:        jsonURL(v.URL),
			Current:    v.Current,
			CommitHash: v.CommitHash,
			CommitTime: commitTimePtr(v.CommitTime),
		}
	}
	return writeJSONFile(outDir, url, out)
}

// IssueRow is one outdated-@uses row, as written to _outdated.json.
type IssueRow struct {
	UserLabel      string           `json:"userLabel"`
	UserURL        string           `json:"userUrl"`
	UseLabel       string           `json:"useLabel"`
	UseURL         string           `json:"useUrl,omitempty"`
	UseFound       bool             `json:"useFound"`
	OldVersion     string           `json:"oldVersion"`
	CurrentVersion string           `json:"currentVersion"`
	Changelog      []ChangelogEntry `json:"changelog,omitempty"`
}

// OutdatedPage is _outdated.json's content - the JSON twin of _outdated.html.
type OutdatedPage struct {
	Major []IssueRow `json:"major"`
	Minor []IssueRow `json:"minor"`
}

func writeOutdatedJSON(outDir string, result *build.Result, byKey map[string]*build.RenderedTarget) error {
	var data OutdatedPage

	for _, issue := range result.Issues {
		row := IssueRow{
			UserLabel:      issue.User.Key(),
			UserURL:        jsonURL(build.RelLink(outdatedURL, build.TargetURL(issue.User.Ref()))),
			UseLabel:       fmt.Sprintf("%s@%s", issue.Use.Key(), issue.Use.Version),
			OldVersion:     issue.Use.Version.String(),
			CurrentVersion: issue.Current.String(),
		}

		if used, ok := byKey[issue.Use.Key()]; ok {
			row.UseFound = true
			row.UseURL = jsonURL(build.RelLink(outdatedURL, build.TargetURL(used.Target.Ref())))
			for i, c := range used.ChangelogHTML {
				entry := ChangelogEntry{Audiences: c.Audiences, HTML: c.HTML}
				if i < len(used.Target.Changelog) {
					entry.Body = used.Target.Changelog[i].Body
				}
				row.Changelog = append(row.Changelog, entry)
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

	return writeJSONFile(outDir, jsonURL(outdatedURL), data)
}

// NavLink is one target's entry in an IndexPage scope group.
type NavLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// ScopeGroup is every target in one scope, as listed in IndexPage.
type ScopeGroup struct {
	Scope   string    `json:"scope"`
	Targets []NavLink `json:"targets"`
}

// IndexPage is index.json's content - the JSON twin of index.html.
type IndexPage struct {
	Groups      []ScopeGroup `json:"groups"`
	OutdatedURL string       `json:"outdatedUrl"`
}

func writeIndexJSON(outDir string, result *build.Result) error {
	groups := map[string][]NavLink{}
	for i := range result.Targets {
		t := result.Targets[i].Target
		label := fmt.Sprintf("%s (%s)", displayName(t), t.Version)
		groups[t.Scope] = append(groups[t.Scope], NavLink{Label: label, URL: jsonURL(build.TargetURL(t.Ref()))})
	}

	scopes := make([]string, 0, len(groups))
	for s := range groups {
		scopes = append(scopes, s)
	}
	sort.Strings(scopes)

	data := IndexPage{OutdatedURL: jsonURL(outdatedURL)}
	for _, s := range scopes {
		links := groups[s]
		sort.Slice(links, func(i, j int) bool { return links[i].Label < links[j].Label })
		data.Groups = append(data.Groups, ScopeGroup{Scope: s, Targets: links})
	}

	return writeJSONFile(outDir, jsonURL(indexURL), data)
}

// ChangelogVersion is one target version introduced on a given day, as
// listed in a ChangelogDay.
type ChangelogVersion struct {
	Scope   string `json:"scope"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// CommitHash is the full (40-hex-digit) hash of the commit that
	// introduced this version, or "" if unknown.
	CommitHash string `json:"commitHash,omitempty"`
	// Kind classifies this version against the immediately preceding known
	// version of the same target - "major", "minor", or "patch" (see
	// model.Diff) - so a client can filter the feed by severity without
	// fetching every entry's own target JSON just to compare version
	// numbers. The very first known version of a target (nothing older to
	// compare against, at least within what internal/history discovered)
	// is classified "major", since a target's initial appearance is always
	// a notable event.
	Kind string `json:"kind"`
}

// ChangelogDay is every version introduced on one UTC calendar day.
type ChangelogDay struct {
	Date     string             `json:"date"` // YYYY-MM-DD, UTC
	Versions []ChangelogVersion `json:"versions"`
}

// ChangelogShard is one changelog/<YYYY-MM>.json file's content.
type ChangelogShard struct {
	Days []ChangelogDay `json:"days"`
}

// ChangelogIndex is changelog/index.json's content: {year: {month: [day,
// ...]}}, with month/day as bare (non-zero-padded) integers - see the
// package doc for why.
type ChangelogIndex map[string]map[string][]int

type shardKey struct{ year, month int }

// diffKindLabel renders a model.DiffKind as the lowercase label the
// changelog feed's Kind field uses. model.DiffNone is unreachable here (two
// distinct entries in rt.Versions never carry equal versions) but maps to
// "patch" rather than panicking or leaving Kind empty, should that ever
// change.
func diffKindLabel(k model.DiffKind) string {
	switch k {
	case model.DiffMajor:
		return "major"
	case model.DiffMinor:
		return "minor"
	default:
		return "patch"
	}
}

// writeChangelogJSON writes one changelog/<YYYY-MM>.json shard per month
// that saw a version introduced, plus changelog/index.json - the sparse
// ChangelogIndex of which shards/days actually have entries, so a client
// knows what to fetch without probing every month.
//
// Only versions with a known CommitTime are included - the same
// best-effort-unknown case every other commit-derived field in this
// package documents; a version whose introducing commit couldn't be found
// has no date to file it under.
func writeChangelogJSON(outDir string, result *build.Result) error {
	shards := map[shardKey]map[string][]ChangelogVersion{}
	index := map[string]map[string]map[int]bool{}

	for i := range result.Targets {
		rt := &result.Targets[i]
		t := rt.Target
		for vi, v := range rt.Versions {
			if v.CommitTime.IsZero() {
				continue
			}
			ct := v.CommitTime.UTC()
			sk := shardKey{ct.Year(), int(ct.Month())}
			dateStr := ct.Format("2006-01-02")

			// rt.Versions is current-first then historic newest-first (see
			// build.versionLinks/addHistoricVersions), i.e. already sorted
			// descending by version - so the entry right after this one is
			// the immediately preceding version, if any is known.
			kind := "major"
			if vi+1 < len(rt.Versions) {
				kind = diffKindLabel(model.Diff(rt.Versions[vi+1].Version, v.Version))
			}

			if shards[sk] == nil {
				shards[sk] = map[string][]ChangelogVersion{}
			}
			shards[sk][dateStr] = append(shards[sk][dateStr], ChangelogVersion{
				Scope:      t.Scope,
				Name:       t.Name,
				Version:    v.Version.String(),
				CommitHash: v.CommitHash,
				Kind:       kind,
			})

			yearStr := strconv.Itoa(ct.Year())
			monthStr := strconv.Itoa(int(ct.Month()))
			if index[yearStr] == nil {
				index[yearStr] = map[string]map[int]bool{}
			}
			if index[yearStr][monthStr] == nil {
				index[yearStr][monthStr] = map[int]bool{}
			}
			index[yearStr][monthStr][ct.Day()] = true
		}
	}

	for sk, days := range shards {
		dates := make([]string, 0, len(days))
		for d := range days {
			dates = append(dates, d)
		}
		sort.Strings(dates)

		var shard ChangelogShard
		for _, d := range dates {
			versions := days[d]
			sort.Slice(versions, func(i, j int) bool {
				if versions[i].Scope != versions[j].Scope {
					return versions[i].Scope < versions[j].Scope
				}
				return versions[i].Name < versions[j].Name
			})
			shard.Days = append(shard.Days, ChangelogDay{Date: d, Versions: versions})
		}

		shardURL := fmt.Sprintf("changelog/%04d-%02d.json", sk.year, sk.month)
		if err := writeJSONFile(outDir, shardURL, shard); err != nil {
			return err
		}
	}

	dateIndex := ChangelogIndex{}
	for year, months := range index {
		dateIndex[year] = map[string][]int{}
		for month, days := range months {
			list := make([]int, 0, len(days))
			for d := range days {
				list = append(list, d)
			}
			sort.Ints(list)
			dateIndex[year][month] = list
		}
	}

	return writeJSONFile(outDir, "changelog/index.json", dateIndex)
}

// writeJSONFile marshals v as indented JSON and writes it to
// outDir/relURL, creating any needed subdirectories - the JSON-tree
// equivalent of renderPage.
func writeJSONFile(outDir, relURL string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("site: marshaling %s: %w", relURL, err)
	}

	dest := filepath.Join(outDir, filepath.FromSlash(relURL))
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("site: creating directory for %s: %w", relURL, err)
	}
	if err := os.WriteFile(dest, b, 0o644); err != nil {
		return fmt.Errorf("site: writing %s: %w", relURL, err)
	}
	if err := writeJSONPFile(dest+".js", relURL, b); err != nil {
		return err
	}
	return nil
}

// writeJSONPFile writes relURL's data a second time, as a plain executable
// "<relURL>.js" sibling calling the fixed global docsweb_jsonp(url, data)
// function - so the changelog tab (see changelogTmpl in templates.go) can
// load it from a file:// page. Opening a static site directly off disk
// (no HTTP server) is a real, expected way to browse it, but fetch/XHR are
// blocked outright under file:// ("CORS request not http"); a classic
// <script src> element is not subject to that restriction (the same reason
// JSONP existed before CORS did), which is why the changelog tab loads
// every JSON file this way instead of via fetch, on both file:// and a real
// HTTP server alike. relURL is passed back to the callback (marshaled
// through encoding/json for correct JS-string-literal escaping) so the
// loader can match a response to the request that asked for it regardless
// of the order several concurrently-loading <script> tags finish in.
func writeJSONPFile(dest, relURL string, data []byte) error {
	urlLit, err := json.Marshal(relURL)
	if err != nil {
		return fmt.Errorf("site: marshaling %s: %w", relURL, err)
	}
	var jsonp bytes.Buffer
	jsonp.WriteString("docsweb_jsonp(")
	jsonp.Write(urlLit)
	jsonp.WriteByte(',')
	jsonp.Write(data)
	jsonp.WriteString(");\n")
	if err := os.WriteFile(dest, jsonp.Bytes(), 0o644); err != nil {
		return fmt.Errorf("site: writing %s: %w", dest, err)
	}
	return nil
}

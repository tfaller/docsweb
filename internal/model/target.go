package model

// Audience names a group of readers a target/section is relevant to.
// "all" is a reserved name meaning every audience.
type Audience string

// AudienceAll is the reserved audience meaning "everyone".
const AudienceAll Audience = "all"

// ChangelogEntry is one @changelog section: what changed for the current
// revision of a target, and (optionally) which audiences it matters to.
type ChangelogEntry struct {
	// Audiences this changelog entry is relevant to. Empty means "the
	// target's whole audience", per the README's @changelog rules.
	Audiences []Audience
	// Body is the raw (pre link/anchor-resolution) Markdown text.
	Body string
}

// Target is one fully-parsed and merged docsweb target: the result of
// concatenating every docsweb block in a file that defines or continues it.
type Target struct {
	// Scope is the dot-joined scope path the target lives in ("" for the
	// root scope).
	Scope string
	// Name is the target's bare name, unique within Scope.
	Name string
	// Version is the target's current version, from @define.
	Version Version

	DisplayName string // @name
	Summary     string // @summary, raw Markdown, optional

	Uses      []TargetRef // @uses references, in source order
	Audiences []Audience  // @audience for the whole target

	Changelog []ChangelogEntry // @changelog entries, in source order

	// Doc is the target's main documentation body (raw Markdown, from
	// @doc sections, concatenated across continuation blocks).
	Doc string

	// SourceFiles lists every file (relative to the scope root) that
	// contributed to this target, in the order they were merged.
	SourceFiles []string
}

// Key returns the scope+name identity of the target (ignoring version).
func (t *Target) Key() string {
	if t.Scope == "" {
		return t.Name
	}
	return t.Scope + "." + t.Name
}

// Ref returns a TargetRef pointing at this target's current version.
func (t *Target) Ref() TargetRef {
	return TargetRef{Scope: t.Scope, Name: t.Name, Version: t.Version}
}

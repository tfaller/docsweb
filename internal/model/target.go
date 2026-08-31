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
	// Scope is the target's own full dot-joined scope path ("" for the root
	// scope), which may descend further than ConfigScope when @define used
	// a leading-dot or fully-qualified sub-namespace (see
	// model.ParseDefineName).
	Scope string
	// ConfigScope is the dot-joined name of the .docsweb.yaml/build scope
	// that physically governs this target's location - what the collecting
	// AddScope call's Options.Scope was. Used to locate that scope's
	// directory (e.g. for git-blame) and to resolve its audienceMap; Scope
	// is what identifies the target itself.
	ConfigScope string
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

	// DefineLine is the 1-based line number, within SourceFiles[0], of this
	// target's @define line - the line that names its current version.
	// Used to attribute a version bump to whoever last changed that line
	// (git blame), per internal/vcs.
	DefineLine int
}

// Pieces returns every Markdown piece a target carries - its @summary,
// @doc, and every @changelog entry's body - in the order anchor uniqueness
// and @link resolution are checked across them.
func (t *Target) Pieces() []string {
	pieces := make([]string, 0, 2+len(t.Changelog))
	pieces = append(pieces, t.Summary, t.Doc)
	for _, c := range t.Changelog {
		pieces = append(pieces, c.Body)
	}
	return pieces
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

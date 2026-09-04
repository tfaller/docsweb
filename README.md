<!--
    @docsweb
    @define readme v1.1.0
    @name README
    @summary
    Project overview, the full annotation grammar spec, and the
    configuration reference - dogfooded as a real docsweb target via the
    Markdown frontend described in its own "Markdown files" section below.
    @changelog
    Documented private `git:` scopes: a central credential registry
    (see the new `auth` package) resolves HTTP credentials for a scope's
    URL, trying each registered provider in order - today, a GitLab CI
    job's own `CI_JOB_TOKEN`, authenticating an `https://gitlab.com/...`
    scope automatically when docsweb itself runs as a GitLab CI job. See
    "Scopes" > "Private git: scopes" below.
-->

# docsweb

Write technical documentation where it belongs. Besides the code.
Crosslink between projects/repos, get a complete documentation.

> [!IMPORTANT]
> For now just a POC - primarily developed with AI

## Documentation

Documentation happens inside the code. Just write a comment in your language and annotate it as docsweb. The first docsweb block inside a given file must define a target. Any following block which does not define one will be concatenated to the previous one, as if they were just one block. A docsweb block and file can define multiple targets. But targets must be unique in their scope. Concatenation happens in the order of definition, from top to bottom. Documentation itself (and changelog) can be written in Markdown.

```typescript

/*
    @docsweb
    @define target v1.0.1
    @name Some cool target (system/module/feature)
    @summary
    Brief summary what this target is about. Optional.
    @uses bla.bla.x@v1.0.0
    @uses xxx@v2.1.0
    @audience dev, tester, user
    @changelog
    @audience user
    Fix types.
    @doc
    This is really important. Document with markdown.
    @docsweb
 */
```

## Annotation grammar

A docsweb block is opened by `@docsweb` and, from there on, read line by line. Only lines that start with one of the fixed tag names (`@define`, `@name`, `@summary`, `@uses`, `@audience`, `@changelog`, `@doc`, `@docsweb`) are treated as tags. Everything else belongs to whichever tag was opened last, verbatim, and is rendered as Markdown. A section simply ends where the next recognized tag begins, or where the block itself ends — there is no separate nesting rule to keep track of.

This means indentation carries no meaning for docsweb itself. Before the block is read, the common leading whitespace of all its lines is stripped once, so the block does not care about the depth it happens to live at in the source file. Whatever indentation remains after that belongs entirely to Markdown - list nesting, code fences and blockquotes work exactly like they do anywhere else, because docsweb never reinterprets them. While inside a fenced (\`\`\`) code block, tag lines are also not recognized, so annotation syntax can be shown as an example inside the documentation itself without being parsed as one.

Because the documentation body has no fixed position in the block, it needs its own tag: `@doc`. Everything after `@doc` is the target's main documentation, until the next tag or the end of the block.

A block ends at an explicit closing `@docsweb`, or otherwise at the natural end of the surrounding comment: the closing `*/` for a block comment, or the first line that isn't a comment for a run of single-line comments (`//`, `#`, ...). The closing tag is optional and mainly useful when a block comment continues with unrelated text afterwards, or when a block should end deliberately before that natural boundary. For single-line comments, an empty comment line (e.g. a bare `//`) continues the run and represents a Markdown paragraph break; only a genuine blank line (no comment prefix at all) or actual code ends it automatically. This keeps multi-paragraph Markdown safe - a paragraph break inside the documentation never gets mistaken for the end of the block.

## Markdown files

Documentation can also live directly in a Markdown file instead of a source-code
comment. The file still opens with one `@docsweb` annotation comment - written as an
HTML comment (`<!-- -->`) so it doesn't show up when the raw Markdown is rendered
elsewhere - but it must be the very first thing in the file, before anything else, and
a Markdown file may define at most one target this way.

`@doc` is not used here, and is rejected as an error if present: once the annotation
comment closes, everything that follows in the file - the actual Markdown body - is
the target's documentation, verbatim.

```markdown
<!--
    @docsweb
    @define target v1.0.1
    @name Some cool target (system/module/feature)
    @summary
    Brief summary what this target is about. Optional.
    @uses bla.bla.x@v1.0.0
    @audience dev, tester, user
    @changelog
    Fix types.
-->

# Some cool target

Document with markdown, right here as the rest of the file.
```

Every other rule from "Annotation grammar" above still applies to the annotation
comment itself - indentation is stripped the same way, and a fenced code block inside
it still suppresses tag recognition. A Markdown file with no leading `@docsweb`
comment at all is simply not a docsweb target - it's left alone as ordinary Markdown,
the same way a source file with no `@docsweb` block anywhere in it defines nothing.

## Audiences
Not all targets/documentation are important for all readers. `@audience` can be used to define for what readers a target or following text block are relevant. An audience is a group of users. Like devs, testers, admins and actual users as in end users / customers. `all` means all.

Every audience name used by `@audience` must be declared in the scope's own `.docsweb.yaml`, under its top-level `audience:` map (see "Scopes" below) - an undeclared name is a build error, not a silently-accepted free-form tag. The reserved `all` name is the one exception; it never needs declaring. For a referenced scope, "declared" follows the audience-mapping rule described in "Scopes": a name that auto-maps or resolves through that scope's `audienceMap` counts as declared.

## Changelog definition

Changelogs are important. Readers should not have to read the old documentation and the new one to understand what changed. The changelog should explain what and why something was changed. Changelog can define audiences to which the change is important. If it was not explicitly mentioned, it is important to the whole target audience. The changelog content itself follows the `@changelog` tag (optionally after its own `@audience` override) and ends at the next tag - typically `@doc` - or the end of the block. The changelog is meant to reflect just the change for the current version, from the previous to the current.

## Linking between documentations

Defining a linking target/anchor. Add a SemVer to inform about breaking changes. SemVer must be exact, no range definition.

Target and scope names are alphanumeric only and case-sensitive - the same applies to audience names. A target name is separated from its version by `@` (`targetName@vX.X.X`). `@audience` takes a comma-separated list of names; whitespace around names and commas is ignored.

`@define` names itself, and can be written in one of three forms:

```
@define targetName vX.X.X
```

A bare name is relative shorthand: it's implicitly scoped under the scope the defining file lives in (see "Scopes" below).

```
@define .subScope.targetName vX.X.X
```

A leading dot is also relative, but lets a target group itself into a sub-namespace within its own scope without having to retype that scope's name.

```
@define scope.subScope.targetName vX.X.X
```

Without a leading dot, a dotted name is taken completely literally as the target's absolute, fully-qualified name - including its own scope. This is validated: it's a hard build error unless it equals, or is a sub-namespace of, the name the containing scope declares for itself (see "Scopes"). This form is never required, but it makes a single `@docsweb` block fully self-describing even read in isolation, with no need to know which scope's `.docsweb.yaml` governs the file it's in.

```
@uses: scope.targetName@vX.X.X
```

An unprefixed `@uses` resolves relative to the referencing target's own scope (which may itself be a sub-namespace, per the two relative `@define` forms above).

Anchor for links. The anchor name must be unique in the containing target
```
Some [Text](@anchor:name) and more Text
```

```
[Text](@link:scope.target@vX.X.X#anchor)
```

`@anchor:` and `@link:` destinations are resolved by a preprocessing step before the surrounding text is handed to the Markdown renderer, so they can be written as plain Markdown link destinations without needing any special support from the renderer itself.

# Usage graph:

@uses is basically a normal @link. But @uses creates a usages graph. For e.g. if the definer changes something, get info on what uses it (transitively) and probably changes it.

## Pipeline
During documentation rendering, it will check that all @link and @uses land at an existing target. Also, uses that are now "outdated" because of a new SemVer major version will get reported and highlighted in a special documentation section. The pipeline generates a static html documentation. Minor changes introduce non breaking changes, mainly new features, so there will be an informational update. Patch changes are used just to fix wording, typos and so on. So they are ignored by the pipeline and by default cause no "outdated" information. Still, uses need to reference a patch to make it explicit what version was used during writing of the documentation.

Building the documentation requires reading every file anyway, so there is no separate collection phase before validation - targets are collected and their `@link`/`@uses` references resolved lazily against that same pass. Defining the same target twice within one scope is a hard error.

For the POC, the output is one page per target, plus one dedicated page for outdated uses. That page links to both the referencing target and the target's new version, showing the old and the new version and the changelog entries in between.

`docsweb build` is the primary command to run a build. `docsweb check` runs the same validation - config/scope collection, `@audience`/`@uses`/`@anchor`/`@link` checks - without rendering anything, so it can be used as a fast local/CI gate before a real build. `docsweb check` also runs one check `docsweb build` doesn't: see "Version bump check" below.

## Version Control

The documentation system knows about Version Control. Because of course, documentation changes over time. Remote scopes will ideally define a branch as ref. So the documentation is e.g. always in sync with the main branch. But with direct access to version control the generated documentation can link to specific git commits. And things like: when was a specific target version introduced can be made visible. And generated changelog entries can be "blamed" to the user who did it. Also, because a changelog only contains info about the current revision, VCS is needed to generate a whole changelog overview/list. Also, VCS makes it possible to diff the actual docs.

### Version bump check

`docsweb check` diffs every target's documentation (its `@name`/`@summary`/`@doc`/`@uses`/`@audience` - everything but `@changelog` and `@define`'s version itself) against a comparison base commit. If it changed, the target must have bumped its `@define` version, and its `@changelog` must have changed too - a version bump with an unchanged changelog leaves readers with no way to tell what changed. A target that didn't exist yet at the comparison base (nothing to diff against) is exempt, and outside of a git repository this check is skipped entirely, same as git-blame author attribution.

Every text comparison the check makes - documentation content and changelog wording alike - ignores incidental whitespace (indentation, line wrapping): rewrapping a comment without changing any word is not a documentation change and does not need a version bump. Beyond just requiring the changelog to change, the new `@changelog` text also can't simply be the old entry with something appended or prepended to it - a mistake AI-generated documentation makes often enough to check for explicitly. Per "Changelog definition" above, a changelog entry describes only the change for the current version; retaining the previous entry's wording alongside the new one is flagged as an error rather than silently accepted.

The comparison base is chosen automatically: inside a GitLab merge-request or GitHub pull-request CI pipeline (detected via their respective predefined environment variables) it's the merge base against the request's target branch, so a long-lived target branch that keeps moving doesn't produce false positives. Outside of such a pipeline it's the current `HEAD`, so a local run compares your working tree (uncommitted edits included) against your last commit. `docsweb check --base <rev>` overrides this with an explicit revision (a commit SHA, branch, tag, or anything else git itself accepts).

## Scopes

A scope declares its own name: `.docsweb.yaml`'s top-level `name:` is that scope's complete,
self-declared identity (dot-joined, e.g. `com.company.project`) - like a Go module's `module` path,
a C# `namespace`, or a Java `package` declaration. It is chosen once, by the scope itself, never
assembled by whoever happens to reference it. `name:` is required on every `.docsweb.yaml`,
including the root config - there is no implicit "unscoped" default.

A parent config wires in a referenced scope with a `scope:` entry, but the entry's key is not
itself the name - it's the name the parent *expects* to find there. At build time the referenced
scope's own `.docsweb.yaml` is read, and its `name:` must equal that key exactly, or the build
fails - the same way `go get` rejects a dependency whose `go.mod` declares a different module path
than the one requested. A referenced scope's name is never appended to its parent's, and it need
not be a path-based local checkout nested under the parent at all - a remote (`git:`) scope stands
entirely on its own, elsewhere. It's an import, not a nesting composition, so the exact same scope
always has the exact same fully-qualified name no matter which config references it. For referenced
scopes, audiences need to be mapped. Audiences with the same name will get auto mapped. But any
others need to be explicit.

```yaml
name: root
audience:
    user:
    tester:
    dev:
    it:
        combine:
            - dev
            - tester
scope:
    pathBased:
        path: relative/path/to/scope/root
    remoteBased:
        git: repoUrl
        path: path/inside/the/repo
        ref: branch
    "parent.child":
        path: some/path
        audienceMap:
            devs: dev
ignore:
    - testdata/
    - "*_test.go"
```

For this to build, `relative/path/to/scope/root/.docsweb.yaml` (the `pathBased` entry's own config)
must in turn self-declare exactly:

```yaml
name: pathBased
```

A `git:` scope is resolved the same way, except its file tree comes from git's own object store
instead of a local directory: the repository at `git` is mirrored *bare* (or, on a later build,
fetched) into a `docsweb-cache` directory next to the root `.docsweb.yaml`, `ref` (a branch, tag,
or commit; the repository's default branch if `ref` is omitted) is resolved to a commit, and
`path` is then resolved inside that commit's tree - read directly out of git, with no worktree
ever checked out to disk. From there on a remote scope behaves exactly like a
local one - its own `.docsweb.yaml` must still self-declare the expected name, targets link and
`@uses` across scopes the same way, and `ignore:` still only applies to the root scope's own
directory (a remote scope's content is someone else's repository, so the root's `ignore:` rules
don't reach into it). `docsweb-cache` is reused and only fetched (not re-cloned) across builds, so
it's worth adding to the root scope's own `.gitignore` and to its `.docsweb.yaml` `ignore:` list.

### Private `git:` scopes

A `git:` URL that needs credentials is cloned/fetched over HTTPS using whatever a central
credential registry can work out for it, without `path`/`ref` (or anything else in
`.docsweb.yaml`) ever needing to name a credential directly. Each registered provider is asked, in
order, whether it recognizes the URL - the first one that does wins; a URL none of them recognize
(the common case: a public repository) is cloned/fetched unauthenticated, exactly as if the
registry didn't exist.

Today's one built-in provider: when docsweb itself is run as a GitLab CI job, cloning/fetching a
`https://gitlab.com/...` scope is authenticated with that job's own `CI_JOB_TOKEN` - the same
short-lived, automatically-provided token `git clone` itself would use inside a `.gitlab-ci.yml`
job - so a build can reference another private project on the same GitLab instance with no
separately provisioned credential at all.

`ignore` excludes files and directories from every scope this config declares, relative to the
config's own directory - useful for keeping generated fixtures, test-only data, or build output
out of the documentation. Rules work like `.gitignore`: blank lines and `#` comments are skipped,
`!` negates a rule (a later rule overrides an earlier one), a trailing `/` matches directories
only, and a pattern is anchored to the config's directory if it starts with `/` or contains a `/`
anywhere but the end - otherwise it matches at any depth. `*`, `?` and `**` work as usual;
`[...]` character classes are not supported.

## After POC

The following parts of the design are not part of the initial POC and are planned for afterwards:

- Automatic discovery of nested `.docsweb.yaml` files and the resulting nested-scope/audience inheritance. The POC only resolves scopes explicitly declared in a single root config.
- Version control integration beyond blame (author attribution) and diffing documentation against a comparison base commit, both implemented - a changelog overview across versions, and browsing historical versions of a target, are not.

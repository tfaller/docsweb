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

## Audiences
Not all targets/documentation are important for all readers. `@audience` can be used to define for what readers a target or following text block are relevant. An audience is a group of users. Like devs, testers, admins and actual users as in end users / customers. `all` means all.

## Changelog definition

Changelogs are important. Readers should not have to read the old documentation and the new one to understand what changed. The changelog should explain what and why something was changed. Changelog can define audiences to which the change is important. If it was not explicitly mentioned, it is important to the whole target audience. The changelog content itself follows the `@changelog` tag (optionally after its own `@audience` override) and ends at the next tag - typically `@doc` - or the end of the block. The changelog is meant to reflect just the change for the current version, from the previous to the current.

## Linking between documentations

Defining a linking target/anchor. Add a SemVer to inform about breaking changes. SemVer must be exact, no range definition.

Target and scope names are alphanumeric only and case-sensitive - the same applies to audience names. A target name is separated from its version by `@` (`targetName@vX.X.X`). `@audience` takes a comma-separated list of names; whitespace around names and commas is ignored.

`
@define targetName vX.X.X
`

`
@uses: scope.targetName@vX.X.X
`

Anchor for links. The anchor name must be unique in the containing target
`
Some [Text](@anchor:name) and more Text
`

`
[Text](@link:scope.target@vX.X.X#anchor)
`

`@anchor:` and `@link:` destinations are resolved by a preprocessing step before the surrounding text is handed to the Markdown renderer, so they can be written as plain Markdown link destinations without needing any special support from the renderer itself.

# Usage graph:

@uses is basically a normal @link. But @uses creates a usages graph. For e.g. if the definer changes something, get info on what uses it (transitively) and probably changes it.

## Pipeline
During documentation rendering, it will check that all @link and @uses land at an existing target. Also, uses that are now "outdated" because of a new SemVer major version will get reported and highlighted in a special documentation section. The pipeline generates a static html documentation. Minor changes introduce non breaking changes, mainly new features, so there will be an informational update. Patch changes are used just to fix wording, typos and so on. So they are ignored by the pipeline and by default cause no "outdated" information. Still, uses need to reference a patch to make it explicit what version was used during writing of the documentation.

Building the documentation requires reading every file anyway, so there is no separate collection phase before validation - targets are collected and their `@link`/`@uses` references resolved lazily against that same pass. Defining the same target twice within one scope is a hard error.

For the POC, the output is one page per target, plus one dedicated page for outdated uses. That page links to both the referencing target and the target's new version, showing the old and the new version and the changelog entries in between.

`docsweb build` is the primary command to run a build.

## Version Control

The documentation system knows about Version Control. Because of course, documentation changes over time. Remote scopes will ideally define a branch as ref. So the documentation is e.g. always in sync with the main branch. But with direct access to version control the generated documentation can link to specific git commits. And things like: when was a specific target version introduced can be made visible. And generated changelog entries can be "blamed" to the user who did it. Also, because a changelog only contains info about the current revision, VCS is needed to generate a whole changelog overview/list. Also, VCS makes it possible to diff the actual docs.

## Scopes

Target scopes are relative and can be nested: scopeA.scopeB.targetName. Also if the scope path contains a .docsweb.yaml that file can itself define new sub scopes. So if the root scope is "a" and the sub .docsweb.yaml defines a "b" scope, the fully qualified scope is "a.b". For sub scopes, audiences need to be mapped. Audiences with the same name will get auto mapped. But any others need to be explicit.

```yaml
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
- Cross-repo scopes (`remoteBased`, cloning/fetching a remote repository). The POC only resolves local, path-based scopes.
- Version control integration beyond the current working directory - blame, historical versions, a changelog overview across versions, and diffing the documentation itself. The POC only ever looks at the current state of the working directory.

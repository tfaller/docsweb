# CLAUDE.md

Guidance for Claude Code when working in this repository.

## Project

`docsweb` (`github.com/tfaller/docsweb`) — a static documentation generator that reads
`@docsweb` annotation blocks out of source-code comments and builds a cross-linked static
HTML site from them. See [README.md](README.md) for the annotation grammar/spec for architecture, implementation status, and recorded design decisions.

Package layout:

```
cmd/docsweb/       CLI entrypoint ("docsweb build")
internal/model/    Version/TargetRef/Target/Audience types
internal/annotation/  Comment scanning + @docsweb block grammar
internal/config/   .docsweb.yaml loading
internal/collect/  Walks a scope, builds the target registry
internal/mdlink/   @anchor:/@link: resolution + goldmark rendering
internal/build/    Orchestrates config -> collect -> resolve -> render
internal/site/     Static HTML generation
```

## Testing

- All tests must be written with [`github.com/stretchr/testify`](https://github.com/stretchr/testify)
  (`assert` and `require`), matching the existing test files in this repo. Do not use plain
  `t.Errorf`/`t.Fatalf` comparisons or another assertion library.
  - Use `require` when a failed check means the rest of the test cannot meaningfully continue
    (e.g. `require.NoError(t, err)` before asserting on the result).
  - Use `assert` for independent checks where the test should keep reporting further failures.
- Write tests for negative/error paths and not-yet-implemented or rejected functionality, not
  just the happy path — e.g. invalid input, unsupported/deferred features (like remote scopes),
  and duplicate/conflicting definitions all need their own test cases asserting the expected
  error, not just the success cases.
- Follow existing conventions: table-driven cases for multiple similar inputs, fixture data
  under a package's `testdata/` directory for anything beyond a short inline string, and
  integration-style tests (see `cmd/docsweb/main_test.go`, `internal/build/testdata/integration`)
  for exercising a full pipeline end to end.
- Run `go test ./...` (and `go vet ./...`) before considering a change complete.
# @docsweb/web

Client-side scripts for docsweb's generated static site, all bundled together
into one `bundle.js`.

## Building

```
npm install
npm run build
```

`npm run build` runs [rolldown](https://rolldown.rs) (see
`rolldown.config.mjs`), which bundles to
`../internal/site/assets/bundle.js` as a plain IIFE - not an ES module:
`<script type="module">` fails to load at all under `file://` in Chromium
browsers, and opening a generated site straight off disk (no HTTP server)
is a supported way to browse it. That file is embedded into the `docsweb`
binary via `go:embed` (`internal/site/assets.go`) and is checked into the
repo, so building or running `docsweb` itself never requires Node.js - only
re-run the build above after changing anything under `src/`, then commit
the regenerated `internal/site/assets/bundle.js` alongside your source
change.

`npm run typecheck` runs `tsc --noEmit` (see `tsconfig.json`) - type-checking
only, no emit; rolldown does the actual bundling/transpiling.

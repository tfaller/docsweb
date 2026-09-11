import { defineConfig } from "rolldown";

// Bundles src/changelog.ts down to internal/site/assets/bundle.js - the
// exact file internal/site/assets.go embeds via go:embed and the generated
// site loads as a plain <script src="bundle.js">. Named "bundle" rather
// than after any one entry: there's only src/changelog.ts today, but more
// client-side code is expected to join it here, all still resolving to
// this one file the site loads. Bundled as an IIFE, not an ES module:
// <script type="module"> fails to load at all under file:// in Chromium
// browsers (a CORS-style restriction distinct from - and, unlike - the
// fetch/XHR one the JSONP loading below already works around), and opening
// a generated site straight off disk is a supported way to browse it.
export default defineConfig({
  input: "src/changelog.ts",
  platform: "browser",
  output: {
    file: "../internal/site/assets/bundle.js",
    format: "iife",
  },
});

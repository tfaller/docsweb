package site

import _ "embed"

// bundleJS is the generated site's client-side JS, precompiled as a plain
// IIFE (not an ES module - <script type="module"> fails to load at all
// under file://, in Chromium browsers, and opening a generated site
// straight off disk is a supported way to browse it). Source lives in the
// sibling web/ npm package; web/package.json's "build" script (rolldown,
// see web/rolldown.config.mjs) bundles it straight to assets/bundle.js,
// which is what's embedded here and checked into the repo - so building/
// running docsweb itself never requires a Node.js toolchain, only
// re-running the JS build after editing anything under web/src/.
//
//go:embed assets/bundle.js
var bundleJS []byte

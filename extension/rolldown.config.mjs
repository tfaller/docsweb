import { defineConfig } from "rolldown";

// Bundles src/extension.ts into dist/extension.js, the ES module entry point
// VS Code loads (package.json "main"). The "vscode" module is provided by
// the extension host at runtime, so it must stay external.
export default defineConfig({
  input: "src/extension.ts",
  platform: "node",
  external: ["vscode"],
  output: {
    file: "dist/extension.js",
    format: "esm",
  },
});

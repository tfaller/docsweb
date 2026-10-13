import { test } from "node:test";
import assert from "node:assert/strict";
import { checkArgs, runCheck } from "./check.ts";

test("checkArgs omits --base when empty", () => {
  assert.deepEqual(checkArgs(".docsweb.yaml", ""), ["check", "--config", ".docsweb.yaml"]);
});

test("checkArgs passes --base when set", () => {
  assert.deepEqual(checkArgs("c.yaml", "origin/main"), ["check", "--config", "c.yaml", "--base", "origin/main"]);
});

test("runCheck reports failure for a non-zero exit", async () => {
  // "node check ..." fails: no such script.
  const res = await runCheck({ executable: process.execPath, configPath: "x", base: "", cwd: process.cwd() });
  assert.equal(res.ok, false);
  assert.notEqual(res.output, "");
});

test("runCheck rejects when the executable is missing", async () => {
  await assert.rejects(
    runCheck({ executable: "/nonexistent/docsweb", configPath: "x", base: "", cwd: process.cwd() }),
  );
});

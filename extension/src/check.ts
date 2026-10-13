import { spawn } from "node:child_process";

export interface CheckOptions {
  executable: string;
  configPath: string;
  base: string;
  cwd: string;
}

export interface CheckResult {
  ok: boolean;
  output: string;
}

/** Builds the argument list for "docsweb check". */
export function checkArgs(configPath: string, base: string): string[] {
  const args = ["check", "--config", configPath];
  if (base !== "") {
    args.push("--base", base);
  }
  return args;
}

/**
 * Runs "docsweb check" in cwd. Resolves with the combined stdout/stderr and
 * whether the CLI exited 0; rejects only if the executable can't be started.
 */
export function runCheck(opts: CheckOptions): Promise<CheckResult> {
  return new Promise((resolve, reject) => {
    const child = spawn(opts.executable, checkArgs(opts.configPath, opts.base), { cwd: opts.cwd });
    let output = "";
    child.stdout.on("data", (d) => (output += d));
    child.stderr.on("data", (d) => (output += d));
    child.on("error", reject);
    child.on("close", (code) => resolve({ ok: code === 0, output }));
  });
}

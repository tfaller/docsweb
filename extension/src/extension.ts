import * as vscode from "vscode";
import { runCheck } from "./check";

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("docsweb");
  context.subscriptions.push(
    output,
    vscode.commands.registerCommand("docsweb.check", () => checkCommand(output)),
  );
}

export function deactivate(): void {}

async function pickWorkspaceFolder(): Promise<vscode.WorkspaceFolder | undefined> {
  const folders = vscode.workspace.workspaceFolders ?? [];
  if (folders.length <= 1) {
    return folders[0];
  }
  return vscode.window.showWorkspaceFolderPick({ placeHolder: "Folder to check" });
}

async function checkCommand(output: vscode.OutputChannel): Promise<void> {
  const folder = await pickWorkspaceFolder();
  if (!folder) {
    vscode.window.showErrorMessage("docsweb: open a workspace folder to run a check.");
    return;
  }

  const cfg = vscode.workspace.getConfiguration("docsweb", folder.uri);
  const executable = cfg.get<string>("executablePath", "docsweb");

  await vscode.window.withProgress(
    { location: vscode.ProgressLocation.Notification, title: "docsweb: checking..." },
    async () => {
      output.clear();
      try {
        const result = await runCheck({
          executable,
          configPath: cfg.get<string>("configPath", ".docsweb.yaml"),
          base: cfg.get<string>("check.base", ""),
          cwd: folder.uri.fsPath,
        });
        output.append(result.output);
        if (result.ok) {
          vscode.window.showInformationMessage(result.output.trim() || "docsweb: check OK");
        } else {
          output.show(true);
          vscode.window.showErrorMessage("docsweb: check failed - see the docsweb output for details.");
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : String(err);
        output.appendLine(msg);
        vscode.window.showErrorMessage(
          `docsweb: could not run "${executable}" (${msg}). Set "docsweb.executablePath" to the docsweb CLI.`,
        );
      }
    },
  );
}

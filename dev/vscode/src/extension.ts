import * as vscode from "vscode";
import {
  LanguageClient,
  LanguageClientOptions,
  ServerOptions,
} from "vscode-languageclient/node";

let client: LanguageClient | undefined;

const DEFAULT_COMMAND = ["go", "tool", "handlers", "lsp"];

function configuredCommand(): string[] {
  const cfg = vscode.workspace.getConfiguration("handlers").get<string[]>("command");
  if (!Array.isArray(cfg) || cfg.length === 0 || !cfg[0]) {
    return DEFAULT_COMMAND;
  }
  return cfg;
}

async function startClient(): Promise<void> {
  const cmd = configuredCommand();
  const cwd = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  const serverOptions: ServerOptions = {
    command: cmd[0],
    args: cmd.slice(1),
    options: cwd ? { cwd } : undefined,
  };

  const clientOptions: LanguageClientOptions = {
    documentSelector: [{ scheme: "file", language: "go" }],
  };

  const next = new LanguageClient(
    "handlers",
    "handlers",
    serverOptions,
    clientOptions
  );
  client = next;
  await next.start();
}

async function restartClient(): Promise<void> {
  if (client) {
    const prev = client;
    client = undefined;
    try {
      await prev.stop();
    } catch {
      // Ignore stop errors if the previous client failed to start.
    }
  }
  await startClient();
}

export async function activate(context: vscode.ExtensionContext): Promise<void> {
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("handlers.command")) {
        void restartClient();
      }
    })
  );

  try {
    await startClient();
  } catch {
    // Allow activation to succeed so configuration changes can restart the server.
  }
}

export function deactivate(): Thenable<void> | undefined {
  if (!client) {
    return undefined;
  }
  return client.stop();
}

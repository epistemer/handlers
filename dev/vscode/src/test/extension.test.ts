import * as assert from 'assert';
import * as path from 'path';
import * as vscode from 'vscode';

suite('handlers Extension Test Suite', () => {
	test('reports duplicate key diagnostic from handlers lsp', async function () {
		this.timeout(30000);

		const handlersBin = process.env.HANDLERS_BIN;
		if (handlersBin) {
			await vscode.workspace
				.getConfiguration('handlers')
				.update('command', [handlersBin, 'lsp'], vscode.ConfigurationTarget.Global);
		}

		const folders = vscode.workspace.workspaceFolders;
		assert.ok(folders && folders.length > 0, 'expected test-fixture workspace folder');
		const fileUri = vscode.Uri.file(path.join(folders[0].uri.fsPath, 'handlers.go'));

		const doc = await vscode.workspace.openTextDocument(fileUri);
		await vscode.window.showTextDocument(doc);

		const diags = await waitForHandlersDiagnostics(doc.uri, 20000);
		assert.ok(diags.length > 0, 'expected diagnostics from handlers lsp');

		const dup = diags.find((d) => d.message.includes('also at'));
		assert.ok(
			dup,
			`expected duplicate key diagnostic ("also at"), got: ${diags.map((d) => d.message).join('; ')}`
		);
		assert.strictEqual(dup.severity, vscode.DiagnosticSeverity.Error);
		assert.strictEqual(dup.source, 'handlers');
	});
});

function waitForHandlersDiagnostics(
	uri: vscode.Uri,
	timeoutMs: number
): Promise<vscode.Diagnostic[]> {
	const existing = vscode.languages
		.getDiagnostics(uri)
		.filter((d) => d.source === 'handlers');
	if (existing.length > 0) {
		return Promise.resolve(existing);
	}

	return new Promise((resolve, reject) => {
		const timer = setTimeout(() => {
			sub.dispose();
			reject(new Error(`timed out after ${timeoutMs}ms waiting for handlers diagnostics on ${uri.toString()}`));
		}, timeoutMs);

		const sub = vscode.languages.onDidChangeDiagnostics((e) => {
			if (!e.uris.some((u) => u.toString() === uri.toString())) {
				return;
			}
			const found = vscode.languages
				.getDiagnostics(uri)
				.filter((d) => d.source === 'handlers');
			if (found.length > 0) {
				clearTimeout(timer);
				sub.dispose();
				resolve(found);
			}
		});
	});
}

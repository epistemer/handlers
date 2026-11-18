# ADR 0004: Spike S3, editors and agents

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-18 |
| Roadmap item | M1.3 |
| Deciders | Claude (the spike); Q6 settled by it |

## Context

Brief §11 puts one rule engine behind every front end: the producer,
`handlers vet`, a language server for editors, and hooks for coding agents.
Before the freeze, S3 had to find out:

- which library the language server should use;
- whether the engine's two passes are fast enough for typing and saving;
- whether Claude Code delivers a plugin's diagnostics and hook answers to the
  model, and whether a second language server can serve `.go` files beside
  gopls (Q6).

Method, in a scratch module (Go 1.27.4, Claude Code 2.1.319):

- A generator wrote a toy handler package: 200 routes in 11 files, with
  `{id}` and two-wildcard paths, `//h:param` and `//h:perm`.
- A prototype engine: a syntax pass over file contents with `go/parser`
  (near-misses, detached directives, duplicate keys, wildcards against
  parameter names, patterns registered into a scratch `ServeMux`), and a type
  pass with `go/packages`.
- A language server prototype: JSON-RPC over stdio with full document sync,
  publishing the syntax pass's diagnostics on open, change and save.
- A plugin with `.lsp.json` for that server and a PostToolUse hook running a
  `vet` prototype, plus a second plugin whose `.lsp.json` runs gopls. Both
  passed `claude plugin validate`.
- Headless runs, `claude -p --plugin-dir …` with Haiku, each asking the model
  to make one edit that duplicates a key and to quote what it was told.

## Measurements

| What | Result |
|---|---|
| gofmt on the directives | `//h:` lines untouched; `//p[id]:label` rewritten to `// p[id]:label`, and a `//` line inserted before the directive block |
| Syntax pass, 200 routes, every file parsed again | 3.3 ms, 0.9 MB, 19k allocations |
| Language server, `didChange` to diagnostics for the whole package | 5.5 ms on average over 20 changes |
| Type pass with `go/packages`, first and second load, the toy package (no imports) | 340 ms and 270 ms |
| Type pass on packages with dependencies (auth's root package; its example shop), first load and warm | 645 ms; 300–345 ms |
| Claude Code, hook | The model received the hook's `reason` as `PostToolUse:Edit hook blocking error …`, with the file, position and message; the edit stayed |
| Claude Code, handlers' server loaded first, gopls second | The model received `✘ [Line 16:1] duplicate key … (handlers)` |
| Claude Code, gopls loaded first, handlers' server second | No diagnostics from handlers |
| Claude Code, gopls alone, an edit with a type error | No diagnostics (gopls starts lazily; the short run ended first) |
| Claude Code's debug log, gopls loaded first | `LSP: extension .go already handled by "plugin:gopls-spike:gopls"; "plugin:handlers:handlers" will not be used for .go files` |

The debug log settles Q6: Claude Code gives each file extension to one
language server, the first plugin's, and says so as a warning.

## Options

- **Library.** `go.lsp.dev/protocol` (v1.0.1) or `github.com/tliron/glsp`
  (v0.2.2), against writing the few messages the server needs: initialize,
  shutdown, the four document notifications, publish diagnostics,
  completion, hover and definition. The prototype's server, framing
  included, is about 150 lines.
- **Q6.** Ship `.lsp.json` in the Claude Code plugin, which either shadows
  the user's gopls or is shadowed by it; ship it as a proxy that runs gopls
  and adds handlers' diagnostics; or ship only the hook.

## Decision

1. **No LSP library.** `handlers lsp` implements the messages it uses over
   its own JSON-RPC framing, in an internal package, with protocol tests. A
   library would add a dependency for less code than it replaces, and the
   hard parts below are ours with or without one. The prototype was a
   measurement, not a draft; the server must:
   - negotiate the position encoding (UTF-8 when the client offers it, else
     UTF-16) and convert columns;
   - give each diagnostic the range of its directive line, not a fixed
     width;
   - declare full document sync, and use the last change of a batch only
     under it;
   - follow the lifecycle: refuse requests after `shutdown`, exit 1 on an
     `exit` without one, answer unknown requests with `MethodNotFound`;
   - handle messages in one goroutine, so its document state needs no lock,
     with the type pass on a worker whose stale results are dropped;
   - convert paths to and from `file:` URIs portably (Windows drive letters,
     escaping);
   - publish for open documents, and clear a file's diagnostics when it
     closes, disappears, or has no more faults;
   - cache the parsed files of a package and reread one only when it
     changes on disk, instead of reading every file on each keystroke;
   - choose files as the build does: build constraints, `_test.go` excluded,
     one package name.
2. **The two passes stand.** The syntax pass runs on every change; the type
   pass on save, debounced, and only over functions that carry directives.
   With real dependencies the type pass costs about a third of a second warm,
   which is fine on save and wrong for typing.
3. **Q6: the Claude Code plugin ships the hook (and, from M6.2, the skill),
   not a language server.** The hook reaches the model with every fault and
   doesn't compete with gopls. A gopls proxy goes to the backlog, to revisit
   if Claude Code lets two servers share an extension. The hook answers with
   `decision: block`, which is how a PostToolUse hook gives the model
   feedback; the edit stays on disk by design, and the model fixes it or
   asks, as it did in the runs. The hook checks only a `.go` file inside the
   working directory it was started in, and Claude Code's `timeout` bounds
   it.
4. `golang.org/x/tools` (for `go/packages`) is a dependency of the command
   and the type pass. This record justifies it.

## Consequences

- Brief §11 and Q6 change: no `.lsp.json` in `dev/claude-plugin`. The VS
  Code extension keeps its language client, since VS Code takes diagnostics
  from any number of extensions.
- M4.3's done-when drops the language server.
- The hook's message starts with a fixed line and gives one `file:line:col:
  message` per fault, as `vet` prints, so the model can act on it.
- The near-miss rule must be narrow: the prototype's pattern also matched
  ordinary comments such as `// Handler serves…`. Brief §5.3 now defines it.
- [Review 0004](../reviews/0004-s3-editors.md) checked this record.

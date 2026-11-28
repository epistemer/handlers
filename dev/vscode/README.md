# handlers (neozp)

VS Code extension for `github.com/epistemer/handlers` (`//h:` route directives in Go files).

## Features

- **Diagnostics, completion, hover, and go-to-definition** via `handlers lsp`
  (syntax pass on open/change, syntax + type pass on save).
- **Syntax highlighting** (TextMate grammar injection for `source.go`):
  - `//h:<name> <value>` (`key`, `method`, `path`, `desc`, `auth`, `perm`, `view`, `search`, `body`); `//h:param <name> [from <key>] [label <text>]`
  - `//h:param <name> label <text>`

## Requirements

Add `github.com/epistemer/handlers/cmd/handlers` as a `tool` in your Go module:

```sh
go get -tool github.com/epistemer/handlers/cmd/handlers
```

Or install `handlers` on your `PATH` and configure `handlers.command`.

## Extension Settings

- `handlers.command`: Command and arguments used to run the language server (default `["go", "tool", "handlers", "lsp"]`).

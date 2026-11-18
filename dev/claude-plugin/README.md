# handlers Claude Code plugin

Runs `handlers vet -hook` as a `PostToolUse` hook on `Edit|Write` to report
faults in Go `//h:` route directives back to Claude Code.

## Requirements

The hook checks the edited `.go` file's package using:

1. `go tool handlers vet -hook` when `github.com/epistemer/handlers/cmd/handlers`
   is declared as a `tool` in the project's `go.mod`, or
2. `handlers vet -hook` when `handlers` is on `PATH`.

If neither is available, the hook exits `0` silently.

## Install

```text
/plugin marketplace add epistemer/handlers
/plugin install handlers@epistemer-handlers
```

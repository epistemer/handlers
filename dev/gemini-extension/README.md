# handlers Gemini CLI extension

Provides `GEMINI.md` context for `//h:` route directives and an `AfterTool` hook
on `write_file|replace` that checks edited `.go` files with `handlers vet -hook`.

## Requirements

The hook checks the edited `.go` file's package using:

1. `go tool handlers vet -hook` when `github.com/epistemer/handlers/cmd/handlers`
   is declared as a `tool` in the project's `go.mod`, or
2. `handlers vet -hook` when `handlers` is on `PATH`.

If neither is available, the hook exits `0` silently.

## Install

```sh
gemini extensions install https://github.com/epistemer/handlers --path dev/gemini-extension
```

# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.2.0] - 2026-11-22

Amendment A4 (ADR 0005), accepted by epistemer: URL builders return links,
for ui's actions and tables. The root module and `authguard`, `openapi` and
`cmd` are released together.

### Added
- `handlers.Link`: `URL`, `String`, `Method`, `Route`, `With(url.Values)`.
  It satisfies ui's `ui.Link`, `interface{ URL() string }`.
- `handlers.Gate(admit)`: a guard's `Admit` check as ui's actions gate. A
  route link asks the check about its route; any other link is admitted.
- `handlers.NewLink`, for generated code.

### Changed
- **Breaking:** every generated URL builder returns `handlers.Link`
  instead of `string`. A use as a string writes `.URL()`:
  `http.Redirect(w, r, adminroutes.Category(id).URL(), …)`; `%s` keeps
  working through `String()`. To upgrade: require v0.2.0, run
  `go tool genx` (the handler package still type-checks against the old
  builders), then add `.URL()` where the compiler asks.

## [v0.1.0] - 2026-11-18

Initial release of the root module (`github.com/epistemer/handlers`) and its
nested modules (`github.com/epistemer/handlers/authguard`,
`github.com/epistemer/handlers/openapi`, `github.com/epistemer/handlers/cmd`).

### Added

- **Core runtime (`handlers`):** `Route`, `Param`, `Surface` (`HTML`, `JSON`),
  `Auth` (`Public`, `Session`, `Permission`), `From`, `WithRoute`, `Guard`,
  `Option` (`WithErrors`, `WithMaxBody`, `WithMiddleware`), `ErrorWriter`
  (`WriteText`, `WriteJSON`, `Status`), sentinel errors (`ErrBadParam`,
  `ErrNotFound`, `ErrTooLarge`, `ErrInvalidRequest`, `ErrInvalidField`),
  `Mount` plumbing (`NewMount`, `Handle`, `HandleHTTP`, `Writer`, `Done`,
  `Param*`, `Segment`, `Rest`, `Text`), and route lookup/search (`Table`,
  `NewTable`, `ByKey`, `Routes`, `Query`, `Hit`, `Search`).
- **genx generators (`genx.meta.yaml`) and `routes/v1` schema:** `routes`
  (`Prefix`, `Routes`, `ByKey`, URL builders, admin `Handler`/`Register`, API
  `Mount`), `labels` (`labels/v1` for `i18n`), and `permissions`
  (`auth/access` `Perm*` variables, enforcing single-package ownership).
- **Test helpers (`handlers/handlerstest`):** `Guard`, `ErrNoSession`,
  `ErrForbidden`, and `AssertPublic`.
- **Auth guard (`handlers/authguard`):** `New(a, ac)` adapting `auth` and
  `auth/access` to `handlers.Guard` with `Prepare` declaration checks,
  `Check` enforcement, and lazy memoizing `Admit`.
- **OpenAPI adapter (`handlers/openapi`):** `Mount` and `Router` adapting
  `oapi-codegen`'s strict `std-http` server to `handlers.Mount` with
  `kin-openapi` request validation, value stripping on error causes, and
  multipart streaming.
- **CLI (`github.com/epistemer/handlers/cmd/handlers`):**
  - `gen`: parses and type-checks `//h:` directives, checks `routes/v1`,
    supports `-out`, `-package`, `-prefix`, `-check`, `-strict`, and `-baseline`.
  - `openapi`: validates OpenAPI 3.0.x documents, extensions (`x-route-key`,
    `x-permission`, `x-auth`, `x-label`), and `#/components/schemas/Error`
    responses, supporting `-out`, `-package`, `-key-prefix`, `-check`,
    `-strict`, and `-baseline`.
  - `vet`: validates handler packages and cross-checks keys, permissions, and
    `ServeMux` patterns across committed `zz_*.routes.json` files, with
    `-strict`, `-baseline`, and `-hook` (`PostToolUse` / `AfterTool`).
  - `lsp`: JSON-RPC language server with syntax diagnostics on edit, type
    checks on save, completion, hover, and `h:entity` go-to-definition.
- **Editor and agent tools (`dev/`):** VS Code extension (`dev/vscode`),
  Claude Code plugin with `vet -hook` and `handlers` skill (`dev/claude-plugin`,
  `.claude-plugin/marketplace.json`), and Gemini CLI extension
  (`dev/gemini-extension`).
- **Documentation and example:** locked contract (`docs/interface.md`),
  API review (`docs/api-review.md`), runnable quickstart (`README.md`,
  `scripts/quickstart.sh`), and `examples/shop`.

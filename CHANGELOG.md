# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- `handlers gen`, `vet` and the language server name a missing URL builder
  when a handler package links to it before it was generated:
  `<pkg>.<Name> doesn't exist yet: generate the route before linking to it`,
  and `<import path> doesn't exist yet: generate the routes before linking
  to them` from `gen` when the routes package doesn't exist yet (amendment A6, a
  clarification; Confab's F4). The refusal itself is unchanged.

## [v0.3.0] - 2026-11-22

Amendment A5 (ADR 0006), accepted by epistemer: a route can set its own body
limit in place of `WithMaxBody`'s.

### Added
- `//h:body <size>` directive on admin handlers and `x-max-body: <size>`
  extension on OpenAPI operations (`^[1-9][0-9]*(B|KiB|MiB|GiB)$`, fitting in
  `int64`, refused on `GET` routes).
- Optional `maxBody` integer field (minimum 1, absent on `GET`) on a route in
  `routes/v1` (`schema/routes.v1.json` and `internal/routesv1.Check`).
- `Route.MaxBody int64` (bytes; `0` uses the registration's `WithMaxBody`
  limit). When `Route.MaxBody > 0`, registration limits the request body to it
  in place of `WithMaxBody` (larger or smaller, and even when `WithMaxBody(0)`
  was given).
- `handlers lsp`: `body` completion (directive name, example sizes, and units
  after a number) and byte-count hover (`20MiB = 20 971 520 bytes`).

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

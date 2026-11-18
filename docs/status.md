# Status log

Newest first. Every working session adds an entry: what landed, the decisions
taken for you, what's next, and what's waiting on you.

## 2026-11-18: M5.2, zp's admin routes through handlers

On a scratch copy: zp's 217 annotated handlers turned into stub methods
with the new directive form (wildcards `{x}`, `//h:param`, `net/http`
signatures), then the real `handlers vet` and `handlers gen`. Counts only.

- **vet** (1.1 s) reported what ADR 0002 predicted: the duplicate key and
  pattern, 4 paths without their slash, the wildcard named apart from its
  parameter, 63 missing descriptions. And one more: a handler named
  `Register`, which the generated package's own `Register` would clash with.
- After fixing those in the copy (renaming that handler `SignUp`), **gen**
  (0.7 s) wrote 216 routes: **the same 216 keys as zp**, none missing either
  way; **212 of zp's 217 URL builders give the same path**, the other 5 being
  zp's 4 slashless URLs (zp builds `/adminproviders/…`) and the duplicate;
  206 permissions and 10 public routes, as the census counted. The routes
  file is 181 KB.
- **Decided for you:** nothing.
- **Next:** M6: README, the skill, the API review and v0.1.0.
- **Waiting on you:** nothing (see the question about `Register` in the
  session's report, which doesn't block M6).

## 2026-11-18: M5.1, the example shop

- **Landed:** `examples/shop`, an app on cfgx, i18n, genx, models (a real
  sqlc producer without `check`, so CI trusts the committed request, as in
  models' example), errors, auth and handlers: admin pages with a page and
  action sharing `orders.manage`, an API from an OpenAPI file through
  oapi-codegen (run by `go:generate`, with a test that its output is
  current) and `apiroutes.Mount`, a search page, settings, and real sign-in
  flows with a code. Its tests cover 401/403/200 on both surfaces, a bad
  parameter's French label, schema failures without echoed values, search
  hiding what a user can't open, `vet -strict` and `AssertGenerated`. Gemini
  4 Hydrogen (High) wrote it (about 950k tokens over two rounds); Claude
  reviewed it and sent back a fake `go version` producer.
- **Decided for you:** oapi-codegen runs from `go:generate`, not genx: it
  doesn't produce a genx domain, so a genx entry would need a fake emit.
- **Next:** M5.2, the comparison with zp on a scratch copy.
- **Waiting on you:** nothing.

## 2026-11-18: M4, the editor and agent tools

- **Landed:** `handlers lsp` (cmd/internal/lsp, no library: framing,
  lifecycle, UTF-8 or UTF-16 positions, syntax pass on change, both passes
  on save with stale type results dropped, diagnostics for open documents
  only, completion, hover, definition for `h:entity`); `dev/vscode`, the old
  extension turned into a thin client of the server, with an extension test
  that sees a duplicate key's diagnostic; `dev/claude-plugin` (the
  PostToolUse hook, falling back from `go tool handlers` to `handlers` on
  PATH to nothing) and the repository's marketplace, both passing `claude
  plugin validate`; `dev/gemini-extension` (Gemini CLI 0.64's `AfterTool`
  hook takes the same payload field and answer, so `vet -hook` serves it
  unchanged). CI gains a VS Code job (the `.vsix` as an artifact) and a
  plugins job. Gemini 4 Hydrogen (High) wrote it from a spec (about 480k
  tokens); Claude reviewed it.
- **Decided for you:** diagnostics span from their column to the end of the
  line; on save the server publishes once, after the type pass.
- **Next:** M5, the example app and the comparison with zp.
- **Waiting on you:** nothing.

## 2026-11-18: M3, the API surface

- **Landed:** `handlers openapi` (every §7.2 check with its message, §7.1's
  parameter mapping, §7.4's routes, sharing gen's encoding, checker,
  `-check` and baseline), the `openapi` module (`Mount`, the `Router` that
  oapi-codegen registers through, kin-openapi validation without a second
  router, multipart streamed, values stripped from causes), and the
  `routes` generator's API files with `Mount`. The app module now runs
  oapi-codegen v2.8.2 through genx and passes ADR 0003's whole table through
  `apiroutes.Mount`: 401 before binding, 400 naming the field without its
  value, 403, 200 with decimal money, 404 and 413 as JSON. Gemini 4
  Hydrogen (High) wrote it from a spec (about 600k tokens); Claude
  reviewed it, the adapter line by line.
- **Decided for you:** amendment A1 (`Mount.HandleHTTP`, `Mount.Writer`:
  plumbing for the openapi module, which apps don't call) and A2 (§3.3's
  example uses `GetSpec`, as oapi-codegen v2.8 deprecates `GetSwagger`).
  `-key-prefix` that isn't a key is a usage error (exit 2).
- **Noticed:** genx requires every producer to emit a domain file, so the
  app's oapi-codegen step declares the spec it reads as its "emit". It works
  but reads wrong; M5.1's example will settle how an app runs oapi-codegen.
- **Next:** M4, the editor and agent tools.
- **Waiting on you:** nothing.

## 2026-11-18: M2.6, `Table` and `Search`; M2 is done

- **Landed:** `NewTable`, `ByKey`, `Routes` and `Search` (interface §11.7,
  §14): one `tr.Search` over `route.` in the request's locale, a match on
  the text or source text ignoring case, `Only`, the guard's `Admit`, and
  `Limit`. The benchmark answers the cost question from the plan: 300 routes,
  a third of them matching, take **0.56 ms** a search (185 KB, 3,317
  allocations) on the development machine, under M2.6's 1 ms budget; the
  bbolt walk dominates, as predicted. Gemini 3.8 Flash (High) wrote it from a
  spec (about 310k tokens); Claude reviewed it.
- **Decided for you:** a request without a locale searches in the
  translator's default locale.
- **Next:** M3, the API surface (the OpenAPI producer, the `openapi`
  adapter, `Mount`), and M4, the editor and agent tools, which can run side
  by side.
- **Waiting on you:** nothing.

## 2026-11-18: M2.5, permissions, `authguard` and `handlerstest`

- **Landed:** the `permissions` generator (`access.Permission` per
  permission, `auth.Unverified` where declared, a key in two routes files
  refused); the `authguard` module (Prepare refuses an undeclared
  permission, Check returns auth's own refusals, Admit memoizes per
  permission under a mutex); `handlerstest` (a fake guard and
  `AssertPublic`). Tests with authtest: 401 without a session, 403 without
  the permission or a verified session, 200 with them, unverified
  permissions, the refusal in the JSON envelope, and the app module wired
  through the generated permissions and `authguard`. Gemini 4 Hydrogen
  (High) wrote it from a spec (about 320k tokens); Claude reviewed it,
  `authguard` line by line.
- **Decided for you:** an unknown `Auth` panics in the guards too (§12.2);
  `Admit` returns no error in this version and logs a non-refusal error
  once, treating it as a refusal, because it checks lazily.
- **Next:** M2.6, `Table` and `Search`.
- **Waiting on you:** nothing.

## 2026-11-18: M2.3 and M2.4, the runtime and the `routes` and `labels` generators

- **Landed:** the runtime package (interface §11.1–11.3 and §12: routes,
  the context, guards, options, writers, statuses, `Mount` for generated
  code, parameter binding and URL escaping), and `genx.meta.yaml` with the
  `routes` generator (admin pages; the API's `Mount` waits for M3) and the
  `labels` generator. Golden files for every parse kind, typed parameters
  with aliases, the root pattern and an empty package; i18n's own `labels`
  generator accepts the emitted files. A nested app module generates,
  compiles and serves through `adminroutes.Register`: 400 with the
  parameter's translated label, 404 under the prefix, the route in the
  context before the guard, panics without a guard, escaped URL builders.
  Gemini 4 Hydrogen (High) wrote it from a spec (about 360k tokens); Claude
  reviewed it.
- **Decided for you:** a text type in a `{name...}` wildcard builds its URL
  with `Rest(Text(p))`; `Surface.String` and `Auth.String` panic on unknown
  values (§12.2); the root module requires genx v0.2.0, for `genxtest` in
  its tests only (apps don't build it).
- **Next:** M2.5 (`permissions`, `authguard`, `handlerstest`), then M2.6
  (`Table` and `Search`).
- **Waiting on you:** nothing.

## 2026-11-18: M2.2, `handlers gen`

- **Landed:** `handlers gen` writes `zz_<package>.routes.json` from a handler
  package: names, aliases (numbered on a clash), integer widths and text
  types from the type pass, permissions deduplicated with their text and
  level, every file checked by `routesv1.Check`; `-check`, `-strict`, and the
  removal warnings against the file on disk or a git ref. `vet` now reads
  every committed routes file and compares keys, permissions and prefixed
  patterns across packages, a vetted package standing in for its own file.
  Golden files, and `genxtest.AssertGenerated` in a nested app module that
  CI runs. Gemini 4 Hydrogen (Medium) wrote it from a spec (about 210k
  tokens); Claude reviewed it.
- **Decided for you:** `gen -strict` with warnings writes nothing, as
  interface §5.1 says (the spec had said otherwise).
- **Next:** M2.3–M2.5, the generators.
- **Waiting on you:** nothing.

## 2026-11-18: M2.1, the rule engine and `handlers vet`

- **Landed:** the command's module `cmd/` (golang.org/x/tools only), with
  `cmd/internal/directive` (the syntax pass and the type pass) and
  `handlers vet` (`-strict`, `-hook`). 30 test packages cover every row of
  interface §5.2 and the two §5.3 warnings that need no baseline, plus
  zp's faults rebuilt with toy names; the syntax pass is fed every prefix of
  a file and never panics. CI runs both modules. Gemini 4 Hydrogen (Medium)
  wrote it from a spec (about 160k tokens); Claude reviewed it, fixed a
  staticcheck finding, and ran it on the spike's 200-route package (0.8 s,
  clean) and on the package Claude Code had broken (both faults found).
- **Decided for you:** `{$}` in `h:path` reports `h:path: "<v>": must not
  contain "{$}"`, a `<reason>` under §5.2's row; a parameter type declared in
  the handler's package reports only that, not also "can't be read".
- **Next:** M2.2, `handlers gen`, which also brings `vet`'s comparison across
  packages through the routes files and the baseline warnings.
- **Waiting on you:** nothing.

## 2026-11-18: M1.5, the interface is locked

- **Landed:** [interface.md](interface.md), locked on your instruction
  ("freeze it"), after
  [review 0005](reviews/0005-interface.md) by Gemini 4 Hydrogen (High) in
  two rounds (about 66k and 132k tokens). M1.4 merged as PR 5.
  - Round 1's 18 findings changed the draft: integer widths, import aliases
    and `source` in `routes/v1` (with checker rules and fixtures); a
    catch-all that answers 404 only, under a prefix; `openapi.Mount` with a
    `register` callback that checks completeness itself; a mapping table for
    API path parameters; generated code that can't be shadowed by parameter
    names; one handler type per package; a mux and `Load` per surface.
  - Round 2 caught a flaw in sharing permissions across packages, so in
    v0.1 a permission belongs to one routes package.
  - One finding rejected: errors renders an `i18n.Ter` argument in the
    request's locale, so `ErrBadParam` takes the parameter's label.
- **Decided for you:** the interface's own choices beyond your decisions,
  locked because you told me to freeze it. You meant less than I first
  recorded: API design stays yours, so the main choices of mine are listed
  for you to confirm or overturn, and ownership.md says so.
- **Next:** M2, the admin surface: the rule engine and `handlers vet`
  (M2.1), the producer (M2.2), the generators (M2.3–M2.5), the runtime
  (M2.6).
- **Waiting on you:** nothing; you kept all ten defaults the same day.

## 2026-11-18: M1.4, the routes/v1 schema

- **Landed in [PR 5](https://github.com/epistemer/handlers/pull/5),** merged: `schema/routes.v1.json`, `internal/routesv1.Check` with its
  rules in `internal/routesv1/rules.md`, and `testdata/routes` (4 valid, 43
  invalid files, each with its exact message). Gemini 3.8 Flash (High) wrote
  it from a rule list in a worktree outside the repository (about 370k
  tokens); Claude reviewed it and added rule 11: a route's name can't take a
  name the generated package declares (`Handler`, `Register`, `Routes`,
  `ByKey`, `L10n`) or a variable of the document.
- **Decided for you:** rule 11, and the agent's three readings of the rule
  list: only `auth: permission` routes count as using a permission; a bad
  wildcard isn't also reported as unmatched; `L10n` and `Perm` variables are
  unique within their own prefix.
- **Next:** M1.5, `docs/interface.md` drafted from the brief, ADRs 0002–0004
  and the rules, for you to lock.
- **Waiting on you:** nothing until that draft.

## 2026-11-18: M1's three spikes

- **Landed:** [ADR 0002](adr/0002-spike-s1-census.md),
  [ADR 0003](adr/0003-spike-s2-api-server.md) and
  [ADR 0004](adr/0004-spike-s3-editors-and-agents.md), each reviewed by
  Gemini 3.8 Flash (High) ([0002](reviews/0002-s1-census.md),
  [0003](reviews/0003-s2-api-server.md), [0004](reviews/0004-s3-editors.md);
  about 160k, 190k and 63k tokens). Argon was cooling down all session.
  - **S1:** all 217 zp admin routes are expressible; the rules catch real
    faults in 7 declarations. The grammar gains camelCase key segments (with
    a Go-name collision check), `true` as a flag value, and slash rules.
  - **S2:** the API uses oapi-codegen's strict `std-http` server, registered
    on the app's `ServeMux` through handlers' adapter, with the route in the
    context, the guard, then kin-openapi's validation, before binding. ogen
    lost on its second router. Validation costs about 20 µs a request.
    OpenAPI 3.0.
  - **S3:** the syntax pass takes 3 ms on 200 routes, and a change gets its
    diagnostics in 5.5 ms; the type pass takes 0.3 s warm with real
    dependencies, so it runs on save. The language server takes no library.
    Claude Code gives `.go` to one language server (its debug log says so),
    so the plugin ships the hook, which reached the model in every run.
- **Decided for you:** everything above; each ADR names what stays yours at
  the freeze (the grammar, what an app writes).
- **Next:** M1.4, the `routes/v1` schema and conformance files, then the
  interface draft for your freeze (M1.5).
- **Waiting on you:** nothing yet; the freeze comes with M1.5.

## 2026-11-18: M0, the repository and the design review

- **Landed:** the repository (go.mod, MIT, CI, ownership delegated today),
  [ADR 0001](adr/0001-routes-domain.md), and
  [review 0001](reviews/0001-design.md) of the design by Gemini 4 Hydrogen
  (High), about 60k tokens (Argon was cooling down). Fourteen findings,
  twelve accepted and two in part; the brief, ADR and roadmap changed with
  them. The biggest: `routes/v1` now carries every Go name its generators
  need and a deduplicated permission list; the generated package never
  imports the handler package; guards fail closed (`perm` beside `public` is
  an error, a bearer operation needs a permission unless `x-auth: session`,
  an operation without `security` is an error); a third-party API server
  must not bring a second path matcher; refusals from auth reach the
  surface's error format through `a.Check`.
- **Decided by you:** the four push-backs of the plan: `//h:param`, `session`
  in place of `root`, the removed-key warning, the extension as an LSP
  client.
- **Decided for you:** the extension is copied without its git history;
  `Guard.Admit` memoizes `ac.Check` per permission (a bulk read in auth only
  if the benchmark asks for it); the `operationId` → key rule (brief §10).
- **Next:** M1's spikes: S1 (zp census), S2 (the API server), S3 (editors
  and agents), then the `routes/v1` schema and the freeze.
- **Waiting on you:** nothing.

## 2026-11-18: the plan

- **Landed:** [brief.md](brief.md) and [roadmap.md](roadmap.md), from a
  planning session that read how zp generates its admin routes (comment
  directives) and its API (a hand-written OpenAPI file). Not yet a git
  repository.
- **Decided by you:** the name `handlers`; comment directives for admin
  routes, hardened; a spec-first API; permissions generated for routes
  through auth; the VS Code extension and agent tools in `dev/`.
- **Decided for you:** the brief's (Claude) rows, all open until the freeze.
  Route meta is dropped in favour of a generated route table that search
  joins by key (brief §9, D10).
- **Next:** M0.1, then the spikes of M1.
- **Waiting on you:** creating `github.com/epistemer/handlers` and granting
  ownership (M0.1). Both done the same day.

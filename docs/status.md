# Status log

Newest first. Every working session adds an entry: what landed, the decisions
taken for you, what's next, and what's waiting on you.

## 2026-11-18: M1.4, the routes/v1 schema

- **Landed in [PR 5](https://github.com/epistemer/handlers/pull/5),** merging once CI is green: `schema/routes.v1.json`, `internal/routesv1.Check` with its
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

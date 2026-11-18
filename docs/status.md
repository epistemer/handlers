# Status log

Newest first. Every working session adds an entry: what landed, the decisions
taken for you, what's next, and what's waiting on you.

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

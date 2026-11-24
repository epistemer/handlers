# handlers — Roadmap to the first release

> Status: **plan accepted by epistemer on 2026-11-18**. Module:
> `github.com/epistemer/handlers`.
>
> | Document | Answers |
> |----------|---------|
> | [ADR 0001](adr/0001-routes-domain.md), from [brief.md](brief.md) | *What* gets generated, and why that way |
> | [interface.md](interface.md) (locked 2026-11-18) | *What* an app writes and gets back |
> | this file | *In what order*, and *when each piece counts as done* |

## What gets built

Two producers, three trivial generators, a runtime package, a guard over auth,
and tools for editors and agents:

```
handler package (//h: directives) ── handlers gen ──┐
                                                    ├─► zz_<pkg>.routes.json (routes/v1)
openapi.yaml ─────────────────── handlers openapi ──┘          │
                                    ├─ handlers: routes       → zz_<pkg>_routes.go
                                    ├─ handlers: labels       → zz_<pkg>.labels.json → i18n: labels
                                    └─ handlers: permissions  → zz_<pkg>_permissions.go
openapi.yaml ── the server generator S2 picks ─► the API's types and server
```

## How to use this document

- **Ownership.** epistemer grants it when the work starts (M0.1), on the
  terms of models' `docs/ownership.md`.
- **IDs.** Every item has an ID (`M2.1`) and fits one PR or one focused
  session. Branch names, PR titles and commit subjects start with the ID.
- **Interface first.** M1 ends by freezing `docs/interface.md`. After that a
  change to what an app writes or gets back is an amendment.
- **Versions.** The first usable release is `v0.1.0`. `v1.0.0` is
  epistemer's call.
- **Sizes.** S is up to 1 day, M is 2–3 days, for one developer without AI
  help. They order the work; they don't promise dates.

## Overview

| Milestone | Goal | Needs |
|-----------|------|-------|
| [M0](#m0--foundation) | Repository, ownership, design record, this plan, reviewed | – |
| [M1](#m1--unknowns-and-the-freeze) | Three spikes; the domain's schema; interface frozen | M0 |
| [M2](#m2--admin-routes) | Directives checked and generated: routes, labels, permissions, runtime | M1 |
| [M3](#m3--the-api) | The OpenAPI producer and the API server on the same domain | M2 |
| [M4](#m4--editors-and-agents) | Language server, VS Code extension, Claude Code plugin, Gemini extension | M2 |
| [M5](#m5--an-example-and-zp) | An example app; a comparison with zp | M3, M4 |
| [M6](#m6--v010) | Docs, the skill, `v0.1.0` | M5 |

M3 and M4 are independent and can run side by side.

## Definition of done (every work item)

1. `go test -race ./...` passes in every module (every `go.mod`) on the whole
   CI matrix.
2. Generated files are golden-tested with `genxtest.Render`, and a second
   run gives the same bytes.
3. Every labels file a test produces passes genx's conformance checks.
4. Every diagnostic has a test with its exact message.
5. If the behaviour differs from interface.md, that document is amended in
   the same PR.
6. A new dependency needs an ADR.
7. Nothing of zp's routes, spec or schema names enters the repository or its
   history.

---

## M0 — Foundation

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M0.1 | epistemer creates `github.com/epistemer/handlers` and grants ownership. go.mod (Go 1.26), MIT, CI like the siblings', `docs/ownership.md` | S | Merged |
| ☑ | M0.2 | Commit [brief.md](brief.md) and this roadmap; ADR 0001 from the brief's §§1–3 | S | Merged |
| ☑ | M0.3 | Cross-model review of ADR 0001 and the roadmap (Argon High). Every finding becomes a change or a recorded rejection | S | Triage in [`docs/reviews/0001-design.md`](reviews/0001-design.md) merged |

## M1 — Unknowns and the freeze

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M1.1 | **S1: zp census.** On a copy of zp in a scratch folder, run every admin route through the draft rules of brief §5: directive shapes, how many fail which check, wildcard conversions and `ServeMux` conflicts, parameter types, auth levels, page/action pairs (Q1) | M | [ADR 0002](adr/0002-spike-s1-census.md) with counts only. Every zp route expressible, or the grammar amended |
| ☑ | M1.2 | **S2: the API server.** ogen, oapi-codegen's strict `std-http` server, and oapi-codegen's types with this package's `routes` template, on a toy spec with zp's shapes (decimal money, multipart, bearer, the error envelope, a `/api/v1` prefix, request validation, OpenAPI 3.0 and 3.1) | M | [ADR 0003](adr/0003-spike-s2-api-server.md) picks one against brief §10 and pins the OpenAPI version. **Kill criterion:** if none meets §10 without patching generated code, or none keeps one path matcher with the route in the context before the guard, `routes` renders the API over oapi-codegen's types |
| ☑ | M1.3 | **S3: editors and agents.** A Go LSP library; a prototype of the engine's two passes (brief §11) timed on a generated 200-route package; the Claude Code plugin's LSP beside gopls and its PostToolUse hook, end to end | S | [ADR 0004](adr/0004-spike-s3-editors-and-agents.md) with the timings; Q6 answered |
| ☑ | M1.4 | `routes/v1`: JSON schema and conformance files (valid, and one invalid file per rule with its message), as genx's `testdata/labels/` | S | Files merged, with a test that reads them |
| ☑ | M1.5 | **Freeze the interface.** `docs/interface.md`: the grammar, every diagnostic with its severity and message, the producers' flags, the generators and their parameters, the files they write, the generated Go, keys and permission defaults, the OpenAPI extensions, the runtime API | M | Locked on epistemer's instruction, 2026-11-18 ([interface.md](interface.md), [review 0005](reviews/0005-interface.md)) |

## M2 — Admin routes

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M2.1 | `internal/directive`, the rule engine, and `handlers vet` | M | Every §5.3 diagnostic tested on toy packages, zp's faults among them |
| ☑ | M2.2 | `handlers gen`, the directive producer, with `-check` and the removed-key warning (`-baseline <git ref>`) | M | `genxtest.AssertGenerated` in a nested module; the warning's tests, against the file and against a git ref |
| ☑ | M2.3 | `routes` generator: interface, `Register`, binding, URL builders, the route table | M | Golden files; httptest of binding, 400, 404 and 405 in both formats, `{$}` at the root, `rest` parameters in URL builders, route in context before the guard, panic without a guard |
| ☑ | M2.4 | `labels` generator | S | Conformance passes; `i18ntest` shows a translation on a route and a parameter |
| ☑ | M2.5 | `permissions` generator, `handlers/authguard` and `handlers/handlerstest` | M | `authtest`: 403 without the permission, 200 with it; `session`, `unverified` and `public`; refusals in the surface's format; a key declared by two packages stops the generator; an undeclared permission stops `Register`; `Admit`; the fake guard and the assertion |
| ☑ | M2.6 | Runtime: declared errors, the HTML and JSON error writers, `Search` | S | Tests; a benchmark of `Search` with its admit check on 300 routes, under 1 ms on the development machine, in the status log |

## M3 — The API

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M3.1 | `handlers openapi`, the spec producer, with its checks | M | Golden `routes/v1` from a toy spec; each check's test |
| ☑ | M3.2 | The server as ADR 0003 decides: prefix from `servers`, the JSON error envelope, guards from the route table, validation, serving the spec | M | httptest of each; the spec's statuses match the answers |
| ☑ | M3.3 | Labels and permissions for API routes through M2's generators | S | Tests as M2.4 and M2.5 |

## M4 — Editors and agents

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M4.1 | `handlers lsp`: diagnostics, completion, hover, go to definition for `entity` | M | Protocol tests |
| ☑ | M4.2 | `dev/vscode`: copy the extension without its history, rename, make it a client of `handlers lsp`, update the grammar | S | CI builds the `.vsix` and runs an extension test (`@vscode/test-electron`) that opens a file with a fault and gets its diagnostic |
| ☑ | M4.3 | `dev/claude-plugin` (the hook; the skill comes in M6.2; no language server, ADR 0004) and `.claude-plugin/marketplace.json` | S | `claude plugin validate` passes in CI; a CI test feeds the hook a PostToolUse payload for a file with a fault and checks its JSON answer |
| ☑ | M4.4 | `dev/gemini-extension` | S | A CI test feeds its hook a payload for a file with a fault and checks the answer |

## M5 — An example and zp

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M5.1 | `examples/shop`: an app on cfgx, i18n, genx, models, errors, auth and handlers, with admin pages and an API | M | CI green on all three OSes; `genxtest.AssertGenerated` |
| ☑ | M5.2 | zp, on a copy in a scratch folder: convert the directives, generate, and compare routes, keys and URLs with zp's; time a run. Moving zp is out of scope | S | Every zp route generated with the same method, path and key, or the difference explained; numbers in the status log |

## M6 — v0.1.0

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | M6.1 | README quickstart: add handlers to an app on genx, i18n and auth | S | A CI job runs the quickstart's commands in a fresh module and builds it |
| ☑ | M6.2 | The `handlers` skill: its source in `dev/claude-plugin/skills/handlers/`, installed to `~/.claude/skills/handlers/` | S | Written against the frozen interface; `claude plugin validate` passes |
| ☑ | M6.3 | `docs/api-review.md`, cross-model review of the release, CHANGELOG, tag `v0.1.0` | S | Tag pushed |

## A5 — a route's own body limit (v0.3.0)

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | A5.1 | [ADR 0006](adr/0006-route-body-limit.md), questions H5–H9, reviewed | S | epistemer accepts or changes it |
| ☑ | A5.2 | The directive, the extension, `routes/v1` `maxBody`, `Route.MaxBody`, registration, `vet`, the language server, the skill | M | An upload route in the example takes more than its siblings, end to end |
| ☑ | A5.3 | Release v0.3.0 with `authguard`, `openapi` and `cmd` | S | Tags checked through the proxy |

## Confab iteration 1 (v0.4.0)

| ✓ | ID | Item | Size | Done when |
|---|----|------|------|-----------|
| ☑ | A6.1 | F4: gen names a builder linked before it was generated (a clarification) | S | Merged (#23) |
| ☑ | A7.1 | [ADR 0007](adr/0007-request-lifecycle.md), questions N1–N11, reviewed ([0008](reviews/0008-request-lifecycle.md)) | S | Locked by epistemer, 2026-11-24 (#24) |
| ☑ | A7.2 | Recovery on by default, `ErrPanic`, `Panic`, `handlers/middleware` (`Recover`, `Redirect`, `HTMX`, `Boosted`, `ErrorPage`, `Flash`) | M | httptest of every delivery and recovery case in ADR 0007 |
| ☑ | A7.3 | `//h:perm <key> <text>`, `<key> from <package>`, `x-permission` the same; `routes/v1` `from`; the permissions generator and `vet` | M | The example's admin takes a permission from its API, end to end |
| ☑ | A7.4 | Release v0.4.0 with `authguard`, `openapi` and `cmd`; the skill | S | Tags checked through the proxy |

## After v0.1.0 (backlog)

1. Breadcrumbs from a `parent` directive.
2. Nav and menu metadata.
3. API keys, once auth covers them.
4. A client SDK pipeline from the one spec.
5. A gopls proxy as the Claude Code plugin's language server, if Claude Code
   keeps giving an extension to one server (ADR 0004).
6. `routes/v1` and a `permissions/v1` domain in genx, when a second package
   uses them.
7. A golangci-lint plugin over the rule engine.

## Risks

| Risk | Likelihood / impact | Mitigation |
|------|---------------------|------------|
| A zp route the grammar can't express, or a pattern `ServeMux` refuses | Medium / medium | S1 before the freeze |
| No OpenAPI generator meets brief §10 | Low / high | S2's kill criterion |
| The language server is slow on a large package | Medium / medium | Only the syntax pass runs on change; S3 measures both passes |
| Too many permissions to manage | Medium / low | S1's count, `//h:perm`, groups in `auth/access` |
| A key rename orphans grants and translations | Medium / high | The removed-key warning; the committed domain file's diff in review |
| genx lacks something a Go producer needs | Medium / low | Friction goes to genx backlog item 3 (a producer kit) |

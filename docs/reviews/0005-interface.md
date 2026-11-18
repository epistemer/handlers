# Review 0005: the interface, before the freeze (M1.5)

| | |
|---|---|
| Date | 2026-11-18 |
| Reviewer | Gemini 4 Hydrogen (High), through agy, in two rounds: the draft (about 66k tokens), then the revision (about 132k) |
| Under review | [interface.md](../interface.md), `internal/routesv1/rules.md`, `schema/routes.v1.json` |
| Triage | Claude, who also locked the interface (delegated by epistemer on 2026-11-18) |

## Round 1: eighteen findings

Sixteen accepted, one in part, one rejected.

| # | Finding | Triage |
|---|---|---|
| 1 | `parse` had no integer width, so binding couldn't refuse an overflow | **Accepted.** `parse` names the width (`int8` … `uint64`) |
| 2 | Parameter types from packages with clashing names needed import aliases a template can't invent | **Accepted.** `routes/v1` gains `alias`; the producer picks them |
| 3 | A catch-all at `/` hijacks the app's mux; two packages on one prefix panic; `405` with `Allow` leaks protected routes | **Accepted.** The catch-all answers 404 only, exists only under a prefix, and one package owns a prefix |
| 4 | `vet` couldn't pair a handler package with its routes file, or know its prefix | **Accepted.** `routes/v1` gains `source` |
| 5 | Two packages couldn't share a permission | **Accepted in part.** v0.1 refuses a permission in two routes packages with a clear error; sharing and external permissions are §16's. Round 2 showed why deduplicating across packages is wrong |
| 6 | `errors.Key1[i18n.TKer]` can't render a label | **Rejected.** errors depends on i18n and renders an `i18n.Ter` argument in the request's locale (its §5.3) |
| 7 | The API adapter: body re-reading, 413 lost as 400, no location for oapi-codegen's errors, `servers` breaking validation, the root pattern, a forgettable `Done`, `WithErrors` ignored | **Accepted.** `Mount` takes a `register` callback and checks completeness itself; `Router.RequestError` and `WriteError` use the configured writer; `ErrInvalidRequest` (no location) and `ErrInvalidField`; validation ignores `servers`; the root maps to `{$}` |
| 8 | No OpenAPI-to-Go mapping for path parameters; `x-go-type` on one breaks URL builders; partial-segment parameters | **Accepted.** A mapping table; other schemas, `x-go-type` on path parameters and partial segments are errors in v0.1 |
| 9 | Parameter names could shadow imports or constants in generated code | **Accepted.** Generated code names parameters `p0`… and aliases every import |
| 10 | A shared `perm`'s text and conflicting levels were undefined | **Accepted.** Text is the readable key when shared; disagreement on `unverified` is an error |
| 11 | `Admit`'s error and `Query.Text` were unclear | **Accepted.** `Admit` errs only on a read failure; `Text` matches texts, not keys |
| 12 | The near-miss `// h key …` form flags prose | **Accepted.** Dropped |
| 13 | OpenAPI: `security: []` with `x-auth`, `x-auth` with `x-permission`, optional security, other methods, name collisions, `-key-prefix` | **Accepted.** Each has an error; `-key-prefix` applies to derived keys only |
| 14 | Several handler types in a package; text methods versus underlying types | **Accepted.** One type per package; text methods win |
| 15 | One `Load` with both carriers lets cookies reach the API | **Accepted.** The wiring gives each surface its own mux and `Load` |
| 16 | Message stability had holes | **Accepted.** Removal warnings and `-check` messages pinned; Go-quoted parts matched by prefix |
| 17 | `Status` unwrapping; the fake guard's errors | **Accepted** |
| 18 | Nits: mutable `Routes`, nil arguments, `entity` naming itself, `param` twice, single-use `x-permission` | **Accepted** |

## Round 2: the revision

Nine points; all accepted.

| # | Finding | Triage |
|---|---|---|
| 1 | Declaring a shared permission once, in the first package by path, breaks a binary that imports only the other and moves the variable when a package is added | **Accepted.** A permission belongs to one routes package (round 1 #5) |
| 2 | Validation must put the body back, and a body over the limit is 413 there too | **Accepted.** §12.1 says both |
| 3 | `RequestError` logs its cause, though writers log only 500s | **Accepted.** It logs the cause at debug level |
| 4 | Errors without a location in validation | **Accepted.** `ErrInvalidRequest` |
| 5 | One alias for two imports, two aliases for one import | **Accepted.** `routes/v1` rule 12, with fixtures (`_` was already refused) |
| 6 | `uintptr` | **Accepted.** Excluded |
| 7 | Empty `x-label`, names equal to variables, path-item parameters, `$ref`s, an empty search | **Accepted** |
| 8 | Without a prefix, `ServeMux` answers 405 with `Allow` | **Accepted** as documented behaviour: the interface says so and advises a prefix |
| 9 | Unpinned `servers` reasons, a new package's baseline, `openapi -check`, `Done` wording, the wiring example | **Accepted** |

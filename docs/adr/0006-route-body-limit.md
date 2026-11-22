# ADR 0006: a route can set its own body limit

| | |
|---|---|
| Status | Accepted by epistemer on 2026-11-22: H6 decided (replace), H5 and H7–H9 on their defaults. Amendment A5, for v0.3.0 |
| Date | 2026-11-22 |
| Roadmap item | ui D8.3; handlers v0.3.0 |
| Deciders | epistemer decides; Claude proposes, from ui's design of uploads (ui's rich-inputs-design.md §4.5, its question R21) and the spike that tried a capped route (its §8.1, finding 10) |

## Context

Every route a registration makes reads at most `WithMaxBody` bytes
(default 1 MiB; §12.1, step 1). The option is per `Register` or `Mount`,
so one surface has one limit.

ui's uploads send a file with its form, to the form's own handler (ui ADR
0012). An admin surface then has one route that takes a 20 MiB scan
among a hundred that take a few kilobytes. Today the app has two ways,
both bad:
- raise the whole surface to the largest upload, so every route accepts
  a body a hundred times larger than it needs;
- split the upload routes into a second registration, which needs a
  second routes package or a prefix of their own.

ui's spike capped a route below two files and posted them: the server
answered 413, as `Status` says (§12.3), and ui's script now answers that
on the form's upload fields. What's missing is a way to give that one
route the limit it needs.

## Options

1. **A directive and an extension per route** (this proposal):
   `//h:body 20MiB`, and `x-max-body: 20MiB` on an OpenAPI operation. The
   value travels in `routes/v1` and the generated `Route`; registration
   applies it in place of the surface's.
2. **A runtime option per route key**, `WithMaxBodyFor("orders.receipt",
   20<<20)`: nothing generated changes, but the size lives away from the
   route, and a renamed key leaves it behind silently.
3. **Nothing in handlers**: the handler wraps its own body. It can't: the
   surface's `MaxBytesReader` runs first, and an inner reader can only
   lower the limit.

## Decision

Option 1.

```go
//h:key orders.receipt
//h:method POST
//h:path /orders/{id}/receipt
//h:body 20MiB
func (h *H) Receipt(w http.ResponseWriter, r *http.Request, id ids.OrderID) error
```

| Directive | Value | Default |
|---|---|---|
| `body` | `^[1-9][0-9]*(B\|KiB\|MiB\|GiB)$`: a positive whole number and a unit, no space, units as written (`512KiB`, `20MiB`); the bytes it names fit in an `int64` (H5) | the registration's `WithMaxBody` |

- **What wins** (H6): a route's own limit replaces the registration's,
  larger or smaller. §12.1's step 1 becomes: if `Route.MaxBody > 0`, the
  body is limited to it; else if the registration's `maxBody > 0`, to
  that; else not at all. So a registration with `WithMaxBody(0)` still
  applies a route's own, and a route can't ask for no limit.
- **The domain** (H7): `routes/v1` gains an optional `maxBody`, an
  integer of bytes, at least 1, absent on a `GET` route (schema and
  `routesv1.Check`, so `vet` refuses a hand-edited file that breaks
  either); `Route` gains `MaxBody int64`, 0 for none of its own. The
  producers write it from the directive or the extension; `-check`
  catches a stale file as for any other field.
- **The API** (H8): `x-max-body`, a string of the same syntax, on an
  operation. kin-openapi reads a JSON body before the handler, so the
  limit applies there too.
- **Diagnostics**:
  - `h:body: "<v>" is not a size: write a positive whole number and B,
    KiB, MiB or GiB, as 20MiB` (also for `0B`, a space, or `mb`);
  - `h:body: "<v>" is larger than a body can be` (past `int64`);
  - `h:body given twice`;
  - `h:body on a GET route, which has no body` (H9);
  - for the spec, the same texts under `x-max-body`, plus
    `x-max-body must be a string, as "20MiB"` for a number or other YAML
    value, each at the operation, as §7.2's errors are.
- **The language server**: `body` joins directive completion, with the
  four units offered after a number; hover shows the bytes (`20MiB = 20 971 520
  bytes`); the syntax pass reports the errors above.
- Nothing else changes: the 413 is still `ErrTooLarge` or an
  `*http.MaxBytesError` through `Status`, and the error writer answers it.

## Questions

| # | Question | Default | Alternative |
|---|---|---|---|
| H5 | The directive's name and syntax | `body`, binary units `B`/`KiB`/`MiB`/`GiB`, as ui and errors write file sizes (1024 per unit) | `maxbody`, or decimal units (`MB`) |
| H6 | Does a route's limit replace the registration's, or only raise it? | **Decided** by epistemer: replace. A route that takes a short form may also ask for less | Only raise: the registration stays a floor |
| H7 | Where the value travels | `routes/v1` `maxBody` and `Route.MaxBody`, so search and tests see it | Only in the generated registration code |
| H8 | The OpenAPI side | `x-max-body` on an operation, same syntax | Admin routes only; the API keeps one limit |
| H9 | `body` on a GET | An error: a GET has no body to limit | Allowed and ignored |

## Consequences

- An upload route says what it takes, next to its path, and every other
  route keeps the small default.
- `routes/v1` and `Route` grow one field; the generators, `vet` and the
  language server learn one directive. Not breaking: an old domain file
  has no `maxBody`.
- interface.md changes in §4.2 (the table), §5.2 (the errors), §6 and the
  language server's section (completion, hover), §7.2 and §7.3 (the
  extension and its errors), §8 (the field), §11.1 (`Route.MaxBody`) and
  §12.1 (step 1, as above), as amendment A5 once accepted.
- Reviewed in this repository's [0007](../reviews/0007-route-body-limit.md).

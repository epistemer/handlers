# Review 0007: a route's own body limit (ADR 0006, draft)

| | |
|---|---|
| Date | 2026-11-22 |
| Reviewer | Gemini 4 Argon (High), through agy (`adversarial_review`), in one run with ui's contract draft (ui's review [0021](https://github.com/epistemer/ui/blob/main/docs/reviews/0021-d8-4-contract.md)); about 76k tokens for both |
| Under review | [ADR 0006](../adr/0006-route-body-limit.md), with interface.md §4.2, §8 and §12.1 |

Two of the run's 18 findings were about this ADR. Both were valid and
changed it.

| # | Finding | Resolution |
|---|---|---|
| 14 | The size's grammar was loose (a space? case?), a large value overflows `int64`, the one diagnostic told someone who wrote `0B` to write a whole number, and `x-max-body` had no diagnostics | An exact pattern; an overflow error; the diagnostic asks for a positive number; the spec's errors, including a non-string value |
| 15 | `Route.MaxBody == 0` and `WithMaxBody(0)` mean different things and the order wasn't written; the domain's rules and the language server were missing from the change list | Step 1 written out; `maxBody` at least 1 and absent on GET in the schema and `routesv1.Check`; the language server's completion and hover |

# Review 0006: A4, builders return a link

| | |
|---|---|
| Date | 2026-11-22 |
| Reviewer | Gemini 3.8 Flash (High), through agy (`adversarial_review`), about 155k tokens |
| Under review | `link.go` and its test, `routes.go.tmpl`, a regenerated golden package, and ADR 0005's decision |
| Scope | `With`, the link's identity and the table behind it, `Gate` over other link types, the generated builders |

There were 7 findings. Claude checked each; five changed the code.

| # | Finding | Verdict | Resolution |
|---|---|---|---|
| 1 | `Gate` admits a type that embeds `Link`, since its type switch sees only `Link` and `*Link` | Valid | `Gate` finds the route through an unexported method, which an embedding type gets by promotion and no other package can fake; tested |
| 2 | `With` puts a query after a fragment, or drops the fragment | Valid | The query goes before a fragment, which is kept; tested |
| 3 | `Route()` shares the table's `Params`, so a caller could change the table | Valid | `Params` is cloned; tested |
| 4 | `With` of values that encode to nothing leaves a `?` | Valid | Removed when the query encodes to nothing; tested |
| 5 | `Link` doesn't marshal to JSON or text | Rejected: not in A4; a response that carries a link writes its `URL()`, as the handler chooses | — |
| 6 | Links from routes copied out of the table aren't equal to the builders' | Rejected: links come from builders, which point into the table; that is what makes them comparable | — |
| 7 | `Gate(nil)` panics only at the first link | Valid | It panics at once |

# Review 0010: the A9 build

Reviewer: Gemini 4 Argon (High), about 80k tokens, on 2026-11-28. It saw the
code diff of #29 (generated files left out except a sample) and ADR 0008's
Decision. Of 8 findings, 6 changed the code, partly or wholly. One was wrong,
and two parts were left as they are.

| # | Severity | Finding | Disposition |
|---|---|---|---|
| 1 | critical | A second `//h:param` for one parameter would be accepted when the first has no label | **Wrong:** `seenParams` is keyed by name, not by the label, and still reports `h:param <name> given twice` |
| 2 | critical | `routesv1`: `from`'s key syntax isn't checked; a source with a bad key or a missing method misleads; older files' `entity` is still checked; diagnostics out of order | **Fixed:** the key syntax (`"<k>" is not a valid key`, a new conformance file), sources only from valid keys, no method message without a method. **Kept:** checking an older `entity` (status, "Decided for you": the field stays valid; generators ignore it) and the order of joined messages |
| 3 | major | Completion after `from` offers routes that can't be sources | **Fixed:** only GET routes without parameters are offered |
| 4 | major | `fillPattern` with `{$}`, a wildcard without a parameter, or an unclosed brace | **Fixed:** `{$}` writes nothing (in `Hit.Link` too); a wildcard without a value or an unclosed brace is `ErrBadParam`. Generated tables can't have either |
| 5 | major | An unknown kind falls through as a string; a Rest that isn't a string; `Bits` 0 | **Fixed:** both are `ErrBadParam`. **Documented:** `Bits` 0 reads as `strconv.IntSize`, as in `strconv` |
| 6 | minor | `//h:param id from label` reads `label` as the key | **Fixed:** it is the syntax error |
| 7 | minor | A source that is both POST and has parameters reports one problem; duplicate keys cascade | **Kept:** the first problem is enough, and the duplicate key is already an error |
| 8 | minor | CRLF lines; `strings.Fields` against the parser's whitespace | **Fixed:** words split on space and tab only. **Already covered:** `splitLines` strips `\r` |

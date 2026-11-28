# Review 0009: A9 draft, a parameter's source and a link from values

Of [ADR 0008](../adr/0008-param-sources.md), with ui's A10 draft (epistemer/ui#56).

Reviewer: Gemini 4 Hydrogen (High), about 74k tokens, on 2026-11-28. Gemini 4 Argon was out of quota for about 6.5 hours. The reviewer saw the diffs of handlers A9, ui A10 and neozp ADR 0002, with excerpts of `route.go`, `link.go`, the remote select and `middleware.Redirect`. It reviewed both drafts together because they are coupled. Of 13 findings, 12 changed the drafts and 1 was already covered.

| # | Severity | Finding | Disposition |
|---|---|---|---|
| 1 | critical | `const Text ParamKind` collides with the function `handlers.Text`, and a value might be escaped twice | **Fixed:** the kinds are `KindString` … `KindText` (P1). `Canon` returns `MarshalText`'s bytes unescaped (`handlers.Text` doesn't escape), and `Table.Link` escapes once |
| 2 | critical | A required checkbox can't submit `false`, and the raw query reaches `Table.Link` | **Fixed:** a `KindBool` field isn't required. `Go` builds `Table.Link`'s values from the bound fields (`true`/`false`) and never passes the raw query on |
| 3 | critical | `translations/{locale}` (a view, one sourced parameter) was classed as an entity route, and so vanished from `/` mode and lost its `q` | **Fixed:** an entity route isn't a view. A view with parameters is a `Form` item in both modes |
| 4 | critical | `Form` and `Go` would expose routes that aren't searchable, aren't GET, or are JSON | **Fixed:** each of these is `ErrNotFound` before the guard is asked |
| 5 | critical | An empty value or a Rest value could build `//host` or `/a//b` | **Fixed:** `Table.Link` refuses an empty value, a value given twice, and a Rest value with a leading or trailing slash or an empty segment. `middleware.Redirect` refused non-local URLs already (`ErrNotLocal`) |
| 6 | critical | `ui/palette` lacks a route lookup and can't put an error on its field | **Partly covered:** `Table.ByKey` exists. Remote fields keep their kind's rule, so errors land per field, and `Table.Link`'s error becomes the whole form's |
| 7 | major | No full-page answers without htmx, and the dialog's 422 is unspecified | **Fixed:** the `palette.Page` option, `PaletteLinks`, and the 422 swap by ui's own rule |
| 8 | major | Prefilling `q` with the text that found the route; a parameter named `q` | **Fixed:** `q` starts empty (P7), and a view with a parameter `q` gets no extra field |
| 9 | major | `int64` in a number input rounds above 2⁵³, and `int` can be 32 bits | **Fixed:** 64 bits get a numeric text field with a 64-bit parse rule (P4), and up to 32 bits get `Int` |
| 10 | major | A source the guard refuses: the chip contradicted the fallback | **Fixed:** the route is left out of `@` mode, is a `Form` in `all` mode, and its field is its kind's |
| 11 | minor | Events from the dialog bubble into the palette; URLs could be built by concatenation | **Fixed:** the dialog's events are ignored by the palette, `[contenteditable]` counts as editable, and URLs are built with `URL.searchParams` |
| 12 | minor | A nil labeler, an unknown `mode`, a malformed item | **Fixed:** an identity labeler, unknown modes read as `all`, and `PaletteItems` panics on misuse |
| 13 | minor | A source only in its own package; the nested module's tag | **Fixed:** P3 says that a source in another package comes later, and the tag is `palette/v0.6.0`. A Rest parameter's type and a malformed `param` were already diagnosed |

# Review 0008: ADR 0007, the request's lifecycle and shared permissions

| | |
|---|---|
| Reviewed | [ADR 0007](../adr/0007-request-lifecycle.md) and its interface diff (amendment A7), draft of 2026-11-24 |
| Reviewer | Gemini 4 Hydrogen (High), about 58k tokens (Argon was cooling down) |
| Triage | Claude, 2026-11-24 |

Sixteen findings. Twelve changed the draft, three in part, one rejected.

| # | Finding | Triage |
|---|---|---|
| 1 | A reference generates nothing, so a binary that mounts the taking package but not the declaring one never declares the permission | **In part.** An import of the declaring package would make two packages that take from each other a cycle. Documented: such a binary links the declaring package; if not, `Register` panics at startup with authguard's existing message (fail closed) |
| 2 | Every response the flash middleware sees consumes the cookie (a `/favicon.ico` 404, a `fetch`); a slow fragment writes back a stale copy | **Accepted.** Only a `text/html` page (below 300, or 400–599) delivers; `Set-Cookie` only when something changed; the docs say to wrap the HTML surfaces only. Two tabs sharing one cookie is stated as inherent |
| 3 | No limit when reading the cookie; unsigned values can be forged | **In part.** A cookie over 4 KiB is ignored undecoded. Unsigned stays the default (N7): `__Host-` keeps other origins out, so only the person's own browser can forge one, for themselves |
| 4 | An error argument is also an `i18n.Ker` and would travel without its arguments; defined types (`EventID`) would fall to `fmt.Sprint` | **Accepted.** Errors are tested before keys, for `msg` and arguments; arguments are kept by kind |
| 5 | `HEAD`, 1xx, `Flush` before the header, a hijack, and a 5xx after `Add` | **Accepted.** `HEAD` neither receives nor delivers; 1xx ignored; `Flush` triggers the decision; nothing is written after a hijack; a 5xx drops what its request added |
| 6 | The recovering writer's `Unwrap`/`Flush`/`Hijack`; dirty headers before the fallback; tests without an opt-out | **Accepted** for the writer and the headers; no opt-out stays (N2), with what a test sees stated |
| 7 | The `from` grammar: a text "from SSO" misread; a text after a reference turning it into a declaration; two packages with one name | **Accepted** for the first two: the word `from` after the key always makes a reference, with nothing after the package, an error otherwise. Same-named packages: the generator says so, and `-package` renames one |
| 8 | `ErrorPage` doesn't log `page`'s error, logs `err` twice on fallback, and says nothing of a partial write | **Accepted** |
| 9 | `ErrNotLocal`, private, makes a bad return address a 500 | **Accepted.** A declared 400, `errors.handlers.not_local` (N8) |
| 10 | Level strings are unchecked across modules; `Add` panics in handler tests without the middleware | **Rejected.** A wrong level is a programming error and panics at once; losing a message silently is worse. Tests wrap with the middleware |
| 11 | `routes/v1` gains a field with meaning without a version bump; a stale permissions file; labels | **In part.** Old checkers refuse an entry without `var` (loud); `cmd` requires the root of the same release; labels never read permissions. Removing a stale generated file is genx's job, checked at build time |
| 12 | The single-route warning reading other packages contradicts "the producer checks only its file"; rule 2's text drifts when a second route joins | **Accepted** for the wording (a warning, not a check). The drift is today's rule; the ADR advises giving a shared permission its text |
| 13 | `Redirect` with nil; `Vary: HX-Request`; a contradiction about boosted forms | **Accepted** for nil (panics) and the wording. `Vary` is left out: `Redirect` answers POSTs, which aren't cached |
| 14 | Step 0 runs before the route is in the context | **Accepted.** Registration passes the key; `Recover` logs method and path |
| 15 | OpenAPI messages would start with `h:perm:` | **Accepted.** `x-permission:` |
| 16 | `Panic.Error` can panic on a value whose `String` panics | **Accepted.** Formatted defensively |

# Review 0001: the design (M0.3)

| | |
|---|---|
| Date | 2026-11-18 |
| Reviewer | Gemini 4 Hydrogen (High), through agy; about 60k tokens. Argon was on a quota cooldown |
| Under review | ADR 0001, [brief.md](../brief.md), [roadmap.md](../roadmap.md), as committed in e9c5b1c |
| Triage | Claude |

Fourteen findings: five critical, five major, four minor. Twelve accepted,
two accepted in part. Every change is in the brief, ADR 0001 or the roadmap
in the same PR as this file.

| # | Severity | Finding | Triage |
|---|---|---|---|
| 1 | Critical | `routes/v1` lacks the Go names the `labels` and `permissions` generators need (permission and label variables, parameter type imports), so they would have to derive names; JSON escaping isn't a safe way to write Go literals | **Accepted.** The domain gains `var` on routes and parameters, `import` and `kind` on parameters, and a deduplicated top-level `permissions` list with variable, text and level (brief §6). Templates write literals with the built-in `printf "%q"` |
| 2 | Critical | The generated package importing the handler package, or parameter types declared there, makes an import cycle with templates | **Accepted.** The generated package never imports the handler package; such parameter types are an error; the conformance check `var _ Handler = (*H)(nil)` is the app's (brief §5.3, §6) |
| 3 | Critical | Fail-closed gaps: `perm` beside `public`/`session`; `unverified` meant two things; a bearer operation without `x-permission` became session-only; missing `security` unspecified; one permission declared by two packages | **Accepted**, except one point. `perm` with `public` or `session` is an error; `unverified` is the route's permission at `auth.Unverified`; a bearer operation needs the route-key permission unless `x-auth: session`; an operation without `security` (own or top-level) is an error; the `permissions` generator reads every routes file and refuses a key two packages declare (brief §5.2, §5.3, §8, §10). *Rejected:* making a single-use `perm` an error. Sharing is optional, and a single-use `perm` stays a warning, since renaming a route's permission is legitimate |
| 4 | Critical | `Search` had no request or guard, and running auth middleware per route would cost a session load per route | **Accepted.** `Guard.Admit(r)` reads the session once and memoizes permission checks; `Search` takes the request and guard; the benchmark covers the whole call, with a budget (brief §8, §9; roadmap M2.6) |
| 5 | Critical | A third-party OpenAPI server brings its own router: two path matchers can disagree; the OpenAPI version was inconsistent; `servers[0].url` may be a full URL | **Accepted.** One path matcher, with the route in the context before the guard, is a criterion and part of S2's kill criterion; S2 pins 3.0 or 3.1; the prefix is the parsed URL's path (brief §10; roadmap M1.2) |
| 6 | Major | `ServeMux` edge cases: `/` under a prefix catches the subtree; 404 and 405 come out as plain text; conflicts across packages and partial wildcards panic; `{path...}` escaping | **Accepted.** `{$}` at the root; a catch-all under the prefix answers 404 and 405 in the surface's format; patterns are validated by registering into a scratch `ServeMux`, and `vet` checks several packages together; `rest` parameters escape per segment (brief §5.3, §7) |
| 7 | Major | The ADR left D6, D7 and D10 open while relying on them; permission texts bypass labels; importing the extension with history; the skill in two milestones | **Accepted.** D6, D7 and D10 are settled by ADR 0001; the permission exception is in the ADR; the extension is copied without history; the skill lands once, in M6.2, in the plugin and `~/.claude/skills` |
| 8 | Major | The removed-key warning can't fire in CI once the file is regenerated; `perm` and parameter keys weren't covered; the `operationId` rule was vague | **Accepted.** `-baseline <git ref>` for CI, covering route, permission and parameter keys; an exact `operationId` rule with collisions as errors, and `x-route-key` to pin a key (brief §5.3, §10). Warning when `x-route-key` is absent: *rejected*, since the `operationId` is already the API's public, stable name |
| 9 | Major | Type-checking on every keystroke is slow; a hook on one file misses package-wide faults | **Accepted.** A syntax pass on change and a type pass on save; the hook runs `vet` on the file's package (brief §11). The risk is raised to medium |
| 10 | Major | DoD skipped nested modules; `handlerstest` had no item; several done-whens couldn't be checked | **Accepted.** Every module in DoD 1; `handlerstest` in M2.5; CI-run checks for M4.2–M4.4 and M6.1; a budget for M2.6; M5.2 compares every route |
| 11 | Minor | Directives detached from their function by a blank line were silently lost; values on flags; parameter order | **Accepted.** The engine scans every comment and reports a detached block; values on flags and an empty `desc` are errors; parameters keep the signature's order (brief §5.2, §5.3) |
| 12 | Minor | The runtime API is underspecified; refusals from auth bypass the JSON envelope | **Accepted.** `authguard` uses `a.Check`, so refusals go through handlers' error writer in the surface's format; the full runtime API is written at the freeze (brief §7, §8) |
| 13 | Minor | `TextUnmarshaler` is usually on the pointer; URL builders need to format the value | **Accepted.** Pointer implements `TextUnmarshaler` and value implements `TextMarshaler` (brief §5.3) |
| 14 | Minor | A new `auth` value within `v1` is unsafe if a consumer falls through to a default | **Accepted.** Generators, `Register` and guards fail closed on unknown values (brief §6) |

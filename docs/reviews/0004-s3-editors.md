# Review 0004: spike S3, editors and agents (M1.3)

| | |
|---|---|
| Date | 2026-11-18 |
| Reviewer | Gemini 3.8 Flash (High), through agy; about 63k tokens |
| Under review | [ADR 0004](../adr/0004-spike-s3-editors-and-agents.md) and the prototype's engine, language server and hook |
| Triage | Claude |

Thirteen findings. The prototype was built to measure, so many findings are
about its shortcuts; each one became a rule the real code must follow,
listed in ADR 0004 or brief §5.3. Nine accepted, two in part, two rejected.

| # | Severity | Finding | Triage |
|---|---|---|---|
| 1 | Critical | The near-miss pattern matches ordinary comments (`// Handler …`) | **Accepted.** Brief §5.3 defines a narrow near-miss rule, with ordinary comments never matching |
| 2 | Critical | Q6 rested on runs where gopls never reported, even alone | **Accepted**, and settled: Claude Code's debug log says `extension .go already handled by "plugin:gopls-spike:gopls"; "plugin:handlers:handlers" will not be used for .go files`. The ADR quotes it |
| 3 | Critical | `decision: block` leaves the edit on disk | **Rejected** as a fault: that is what a PostToolUse hook's feedback is, and the model fixed or asked in every run. The ADR says so |
| 4 | Critical | The type pass checked every documented function, and didn't guard a nil `TypesInfo` | **Accepted.** Only functions with directives; a package that doesn't type-check reports that, not a panic |
| 5 | Critical | The hook trusted the payload's path | **Accepted in part.** The payload comes from Claude Code, but the hook checks only a `.go` file inside its working directory and reports decode errors |
| 6 | Major | The type-pass timing had no imports; the server read every file on each keystroke | **Accepted.** Measured with dependencies: 645 ms on the first load, 300–345 ms warm, fine on save. The server caches parsed files |
| 7 | Major | The hand-written server breaks the protocol: positions, ranges, lifecycle, concurrency, unknown methods | **Accepted in part.** No library still: the same work falls to us with one. ADR 0004 lists what the server must do, each with a protocol test |
| 8 | Major | Diagnostics for unopened files, never cleared on close or delete | **Accepted** |
| 9 | Major | Parameter matching, empty keys and the hard-coded prefix were wrong | **Accepted in part.** An empty key is "missing key", never a duplicate; the prefix comes from the producer flag; parameters are read after `w` and `r` whatever their grouping. *Rejected:* that handlers may take parameters without a wildcard. Every parameter after `w` and `r` is a path parameter by design (brief §5.3) |
| 10 | Major | No timeouts | **Accepted in part.** The hook's `timeout` in `hooks.json` bounds it; the server never blocks on the type pass (it runs on a worker) |
| 11 | Minor | `file:` URIs weren't portable | **Accepted** |
| 12 | Minor | Read errors and a missing argument were ignored | **Accepted** |
| 13 | Minor | Build constraints and several package names in a directory | **Accepted.** Files are chosen as the build chooses them |

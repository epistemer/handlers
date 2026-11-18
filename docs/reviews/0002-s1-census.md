# Review 0002: spike S1, the census (M1.1)

| | |
|---|---|
| Date | 2026-11-18 |
| Reviewer | Gemini 3.8 Flash (High), through agy; about 160k tokens (it also read the scratch copy the census ran on) |
| Under review | [ADR 0002](../adr/0002-spike-s1-census.md) and its changes to the brief |
| Triage | Claude |

Eight findings. Seven accepted, one in part.

| # | Severity | Finding | Triage |
|---|---|---|---|
| 1 | Critical | The faulty routes overlap: 7 distinct declarations, not 2 + 3 + 4 | **Accepted.** The ADR gives the union and its breakdown |
| 2 | Major | `false` was never written, means the same as leaving the flag out, and invites a reader who checks only presence | **Accepted.** A flag takes no value or `true` |
| 3 | Major | camelCase and snake case keys can give one Go name; the new grammar dropped segments that start with a digit | **Accepted.** A collision of generated Go names is an error, and segments may start with a digit. *Rejected:* forcing each segment to be purely one style, since the collision check already covers what the mix breaks |
| 4 | Major | `ServeMux` can't see a missing slash once it's concatenated; how prefix and path compose, and who writes `{$}`, were unsaid | **Accepted.** The method says slashless paths were counted on their own; the ADR and brief define the composition, and the generator writes `{$}` |
| 5 | Major | Not measured: capitals in static segments, trailing slashes, `HEAD` and `OPTIONS` | **Accepted** in part. Measured: 18 paths with capitals, which stay allowed; no trailing slash but the root's, and a trailing slash elsewhere is now an error. `HEAD` is served by `ServeMux` with `GET`, and `OPTIONS` (CORS) is middleware, so neither needs a directive value |
| 6 | Minor | 207 guarded routes include the duplicate: 206 permission keys | **Accepted** |
| 7 | Minor | Grouping by "the key minus its last segment" is inconsistent | **Accepted.** The ADR no longer proposes a grouping; that's the grant screen's business |
| 8 | Minor | Empty and omitted `desc` were conflated | **Accepted.** Measured: all 63 are omitted, none empty |

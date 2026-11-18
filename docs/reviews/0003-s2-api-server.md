# Review 0003: spike S2, the API server (M1.2)

| | |
|---|---|
| Date | 2026-11-18 |
| Reviewer | Gemini 3.8 Flash (High), through agy; about 190k tokens |
| Under review | [ADR 0003](../adr/0003-spike-s2-api-server.md), the prototype test and the toy spec |
| Triage | Claude |

Nine findings. Six accepted, two in part, one rejected.

| # | Severity | Finding | Triage |
|---|---|---|---|
| 1 | Critical | kin-openapi reads the whole body before any limit: an unbounded body exhausts memory | **Accepted.** The adapter wraps the body in `http.MaxBytesReader` before validation |
| 2 | Critical | kin-openapi's messages quote the failing value, a password included, and the prototype wrote them to the client | **Accepted.** People see the parameter or field and the rule; logs get no value either |
| 3 | Major | Multipart bodies are buffered whole by validation, then parsed again | **Accepted.** `ExcludeRequestBody` for multipart: parameters are validated, the body streams to the handler, the limit applies |
| 4 | Major | `ServeMux` sends `HEAD` to a `GET` operation | **Rejected** as a fault: `HEAD` is `GET` without a body (RFC 9110), admin pages get the same treatment, and the guard and validation run. The ADR says so |
| 5 | Major | Parameters declared on the path item weren't collected | **Accepted** |
| 6 | Major | The decision ignored kin-openapi's double decode; ogen would be ~40× faster | **Accepted in part.** Measured both: ogen 15–17 µs, oapi-codegen without validation 15–19 µs, with kin-openapi 35–38 µs, an empty handler 5–6 µs. The ~20 µs is recorded and accepted for database-bound handlers; generated validators are the fix if it matters. The 40× figure was wrong |
| 7 | Minor | OpenAPI paths `ServeMux` can't route: hyphenated parameter names, two parameters in a segment, `allowReserved` | **Accepted.** `handlers openapi` refuses them |
| 8 | Minor | Nil path item or operation panics without a message | **Accepted.** Registration panics with the pattern |
| 9 | Minor | The 3.1 and size claims were loose | **Accepted in part.** 3.1 waits for oapi-codegen, and the ADR names 3.0's schema rules; the sizes are marked as not like for like |

# ADR 0003: Spike S2, the API's server generator

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-18 |
| Roadmap item | M1.2 |
| Deciders | Claude (the spike); what an app writes is epistemer's at the freeze (M1.5) |

## Context

The API is spec-first (D3). A third-party generator writes its typed server,
and handlers adds the route table, guards, labels, permissions and error
format (brief §10). The server must run on `net/http` with **one path
matcher**, have the route in the context before the guard, send errors
through handlers' JSON writer, validate requests against the spec, map money
to a decimal type, and accept multipart bodies. The kill criterion: if no
generator meets that without patching generated code, `routes` renders the
API's registration over oapi-codegen's types (option 3).

Method: a toy spec in a scratch module, OpenAPI 3.0.3, with a top-level
bearer requirement, a public operation (`security: []`), a session-only one
(`x-auth: session`), an `int64` path parameter, a query enum and bound, a JSON
body with `minLength` and `minItems`, a multipart upload, decimal money, the
shared `Error` response, and `servers` set to `https://shop.example/api/v1`.
Generated with ogen v1.25.0 and oapi-codegen v2.8.2; kin-openapi v0.153.0
for validation. Go 1.27.4.

## Options

1. **ogen.** Generates a `Server` with its own router, a `SecurityHandler`
   called per operation before parameters are decoded, then middleware, then
   the handler. It has options for a path prefix, error handler, not-found
   and method-not-allowed handlers, its own JSON encoder, and generated
   validators that also check responses. Money maps to a decimal through
   `x-ogen-type`, not the more common `x-go-type`. About 94 KB of generated
   Go for five operations, with the client turned off.
   - Its router sits behind a subtree mount (`mux.Handle("/api/v1/", srv)`),
     so `ServeMux` doesn't match operations: a second path matcher.
   - A public operation never reaches the `SecurityHandler`, so the route goes
     into the context through middleware, after decoding.
2. **oapi-codegen, strict `std-http` server.** About 27 KB for the same spec.
   `HandlerWithOptions` registers every operation on a `BaseRouter` that only
   has to implement `HandleFunc` and `ServeHTTP`, with a full pattern
   (`GET /api/v1/orders/{orderId}`), and reads parameters with
   `r.PathValue`. Error handler hooks: `ErrorHandlerFunc`,
   `RequestErrorHandlerFunc`, `ResponseErrorHandlerFunc`. `x-go-type` gives
   `decimal.Decimal`. Parameters are bound before its middleware runs, and it
   has no request validation of its own.
3. **oapi-codegen's types, with registration rendered by `routes`.** The
   most glue: binding parameters and decoding bodies would be ours to write.

## Measurements (option 2 with an adapter)

handlers passes its own `BaseRouter`: an adapter whose `HandleFunc` finds
the route for the pattern in the generated table (no route panics: fail
closed) and registers the operation on the app's `ServeMux` behind this
chain: route into the context, the guard, request validation with
kin-openapi's `openapi3filter.ValidateRequest` given the operation directly
(no kin router), then oapi-codegen's binding and the handler.

| Request | Answer |
|---|---|
| bad path parameter, no session | 401: the guard runs before binding |
| bad path parameter, with the permission | 400 |
| without the permission | 403 |
| valid; the handler reads the route from the context; money as decimal | 200, `"total":"3.1"` |
| public operation, no session | 200 |
| body below `minLength`, `minItems` below 1, query value outside the enum | 400 each |
| unknown path; wrong method | 404 and 405 from `ServeMux` as plain text (brief §7's catch-all answers them in the API's format) |

One `POST` with a two-item JSON body, through `httptest` on the
development machine (three runs of two seconds each):

| Path | Per request | Allocations |
|---|---|---|
| An empty handler on `ServeMux` (what `httptest` itself costs) | 5–6 µs | 15 |
| ogen: routing, security, generated validation, decoding, encoding | 15–17 µs | 59 |
| oapi-codegen strict, through the adapter, without validation | 15–19 µs | 51 |
| oapi-codegen strict, through the adapter, with kin-openapi's validation | 35–38 µs | 108 |

kin-openapi validates by decoding the body into `any` and walking the
schema, and oapi-codegen then decodes it again into its types: that second
decode is the 20 µs difference. Generated validators, as ogen's, avoid it.

kin-openapi's validation messages include the schema and the value. The
JSON writer has to keep them for logs and show people a public
`errors.Invalid` message.

## Decision

**Option 2:** oapi-codegen's strict `std-http` server, registered through
handlers' adapter. The kill criterion isn't met. Q5 is answered: `routes`
doesn't render the API's registration. For a `json` surface it renders the
route table keyed by pattern and the adapter's wiring, and the app passes
oapi-codegen's handler through it.

- The OpenAPI version is **3.0**, which oapi-codegen supports; 3.1 waits for
  it. That means 3.0's schema rules (`nullable`, boolean
  `exclusiveMinimum`).
- Requests are validated by kin-openapi against the spec oapi-codegen
  embeds, with the operation the adapter already knows, so there is one path
  matcher: the app's `ServeMux`.
- The order on every API operation is route, guard, validation, binding,
  handler: an unauthenticated caller learns nothing from a malformed request.

ogen loses on the one-matcher rule and on registration shape: with option 2,
admin pages and API operations go through the same `ServeMux` and the same
chain, which is D6's point. Its generated validators are the better part of
it: option 2 pays about 20 µs per request for runtime validation. Against
handlers that read a database, that is accepted; if it ever matters,
generated validators for the adapter are the fix, and the chain doesn't
change.

The adapter has these rules, from the review:

- It limits the body (`http.MaxBytesReader`, a per-surface option) before
  validation reads it.
- Multipart bodies aren't read by validation (`ExcludeRequestBody`), only
  their parameters, so a handler can stream an upload; the limit still
  applies.
- Path parameters come from the path item and the operation.
- The response to a failed validation names the parameter or field and the
  rule, never the value; kin-openapi's full message, which quotes the value,
  doesn't reach people, and logs get the field and rule only, so a password
  that fails `minLength` is logged nowhere.
- A pattern without a route, a path the spec doesn't have, or an operation
  missing from its path item panics at registration, with the pattern.
- `HEAD` reaches a `GET` operation, as `ServeMux` does for admin pages; the
  guard and validation run as for `GET`.

## Consequences

- New dependencies, for the `json` surface only: oapi-codegen v2 (a tool, and
  its `runtime` module in the app) and kin-openapi. They belong in a nested
  module or a separate package so an app with only admin pages doesn't pull
  them in; the freeze decides which.
- The adapter is part of handlers' runtime API, written at the freeze.
- `handlers openapi` refuses what `ServeMux` can't route: a path parameter
  whose name isn't a Go identifier, two parameters in one segment, and
  `allowReserved`.
- Generated sizes (94 KB for ogen, 27 KB for oapi-codegen) aren't like for
  like: ogen's include its validators and router, which oapi-codegen leaves
  to kin-openapi and `ServeMux`.
- M3.2's done-when uses this record's table as its tests.
- [Review 0003](../reviews/0003-s2-api-server.md) checked this record.

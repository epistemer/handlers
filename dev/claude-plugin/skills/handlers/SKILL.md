---
name: handlers
description: How to use handlers (github.com/epistemer/handlers), neozp's route package, with //h: directives for admin pages, OpenAPI 3.0.x specs for JSON APIs, genx generators (routes, labels, permissions), authguard, the openapi adapter, Table.Search, handlerstest, and editor/agent tools. Use it when writing or checking //h: route directives, setting up genx.yaml with handlers gen/openapi/vet, wiring adminroutes.Register or apiroutes.Mount with authguard and oapi-codegen, handling handlers errors (ErrBadParam, ErrNotFound, ErrTooLarge, ErrInvalidRequest, ErrInvalidField, ErrPanic), recovering panics, redirecting with or without htmx, rendering error pages or carrying a flash message across a redirect (handlers/middleware: Recover, Redirect, HTMX, Boosted, ErrorPage, Flash), sharing a permission between routes packages (//h:perm <key> from <package>) or giving one its text, implementing route search with Table.Search, testing routes with handlerstest and authtest, or configuring handlers lsp and hooks.
---

# Using handlers

Written for **handlers v0.4.0** (2026-11-24), with `authguard/v0.4.0`,
`openapi/v0.4.0`, `cmd/v0.4.0`, `genx v0.2.0`, `i18n v0.3.1`, `errors v0.1.0`,
and `auth v0.3.0`. Look at `go.mod`; if newer, read `CHANGELOG.md`.
`docs/interface.md` is the locked contract and `examples/shop` is a complete
app with end-to-end tests; **the contract is `docs/interface.md`; when this
skill and that file disagree, the file wins**. Load the `genx`, `i18n`,
`errors`, and `auth` skills too.

## The model

- **Two producers, one domain, three generators.** `handlers gen` reads
  `//h:` doc-comment directives on Go handler methods; `handlers openapi`
  reads an OpenAPI 3.0.x spec. Both emit `zz_<pkg>.routes.json` (`routes/v1`).
  From it, `genx` runs `handlers: routes` (`zz_<pkg>_routes.go`),
  `handlers: labels` (`zz_<pkg>.labels.json` -> `i18n: labels` ->
  `zz_<pkg>_i18n.go`), and `handlers: permissions` (`zz_<pkg>_permissions.go`).
- **`net/http` `ServeMux` only.** No third-party router. Every registered
  route runs in fixed order: panic recovery (v0.4.0, always on) -> body
  limit (`WithMaxBody`) -> `Route` into
  context (`handlers.From`) -> `Guard.Check` (unless `public`) ->
  `WithMiddleware` -> API request validation (JSON surface) -> path parameter
  binding (HTML surface) -> handler.
- **Fail closed.** `Route` enters `r.Context()` before `Guard.Check`.
  `Register` and `Mount` panic on a nil mux/handler, a nil guard when any
  route is not `public`, a `Guard.Prepare` error, or an unhandled route.
  Under a prefix, `Done` registers `<prefix>/` answering `ErrNotFound`
  (404, no `Allow`).

## Directives (admin HTML surface)

A directive is a doc-comment line `//h:<name>` or `//h:<name> <value>`: no
space after `//`, lower-case name, one space before value, trimmed to end of
line. Handlers are exported methods on **one** receiver type per package:

```go
//h:key orders.detail
//h:method GET
//h:path /orders/{id}
//h:desc Order detail
//h:perm orders.manage
//h:view
//h:entity orders.list
//h:param id label Order ID
func (h *H) Order(w http.ResponseWriter, r *http.Request, id ids.OrderID) error
```

| Directive | Value | Default |
|---|---|---|
| `key` | dot-separated `[a-z0-9][A-Za-z0-9_]*` | required |
| `method` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` (any case) | required |
| `path` | starts `/`, no trailing `/` (unless `/`), `{name}` and trailing `{name...}`, no `{$}` | required |
| `desc` | non-empty text | readable `key`, with warning |
| `auth` | `public`, `session`, `unverified` | route's permission at `Verified` |
| `perm` | `<key>`, `<key> <text>` (the permission's text), or `<key> from <package>` (a permission another routes package declares); only when `auth` is default or `unverified` | route's `key` |
| `view` | none, or `true` | off (`false`) |
| `search` | none, or `true` | off (`false`) |
| `entity` | another route's `key` in the same package | none (`""`) |
| `body` | `^[1-9][0-9]*(B\|KiB\|MiB\|GiB)$` (`512KiB`, `20MiB`, fits in `int64`, not on `GET`) | registration's `WithMaxBody` |
| `param` | `<name> label <text>` (`<name>` is a path parameter, once each) | readable parameter name |

- **Parameter types** (bound in signature order after `w` and `r`): `string`,
  `bool`, signed/unsigned integers except `uintptr`, named types over those,
  or `T` with `T` implementing `encoding.TextMarshaler` and `*T` implementing
  `encoding.TextUnmarshaler` (text wins if both). `{name...}` must be a
  string type. Types declared in the handler package itself are refused.
- **Errors** (`<file>:<line>:<col>: <message>`, exit 1, writes nothing):
  - Unknown directive: `unknown directive h:<name>`.
  - Near-miss (`// h:...`, `//H:...`, `//p[id]:label X`): `not a directive: write "<fixed>"`.
  - Duplicate directive: `h:<name> given twice` / `h:param <name> given twice`.
  - Bad values: `h:method: "<v>" is not GET, POST, PUT, PATCH or DELETE`,
    `h:auth: "<v>" is not public, session or unverified`,
    `h:<view|search> takes no value or true, not "<v>"`,
    `h:desc is empty`, `h:<key|perm|entity>: "<v>" is not a key`,
    `h:path: "<v>" must start with "/"`, `must not end with "/"`, `must not contain "{$}"` / `ServeMux` reason,
    `h:body: "<v>" is not a size: write a positive whole number and B, KiB, MiB or GiB, as 20MiB`,
    `h:body: "<v>" is larger than a body can be`,
    `h:body on a GET route, which has no body`,
    `h:param: write "h:param <name> label <text>"`, `h:param: no parameter <name>`.
  - Auth/perm/entity rules: `h:perm needs a permission route, not auth <auth>`,
    `permission "<k>": <key a> is unverified and <key b> isn't`,
    `h:entity names the route itself`, `h:entity: no route with key "<k>"`.
  - Signatures/types: `directives on <A> and <B>: one handler type per package`,
    `missing h:<key|method|path>`,
    `directives must be the doc comment of a handler method`,
    `<Name> isn't a handler: <not a method|not exported|the first parameters must be http.ResponseWriter and *http.Request|it must return error>`,
    `wildcard {<name>} has no parameter`, `parameter <name> has no wildcard in <path>`,
    `parameter <name>: a {<name>...} wildcard needs a string type`,
    `parameter <name>: type <T> can't be read from a path`,
    `parameter <name>: type <T> is declared in this package; declare it in another so the generated package can import it`,
    `the package doesn't type-check: <first error>`, or, when the cause is a
    link made before generating, `<pkg>.<Name> doesn't exist yet: generate the route before linking to it`
    and `<import path> doesn't exist yet: generate the routes before linking to them`.
  - Duplicates/collisions: `key "<k>" also at <file>:<line>`,
    `<METHOD> <prefix><path> also at <file>:<line>`,
    `<METHOD> <prefix><path> conflicts with <METHOD> <prefix><path> at <file>:<line>`,
    `keys "<a>" and "<b>" both give the Go name <Name>`,
    `<Name> is a name the generated package declares`,
    `<Name> is also the label variable of <key>`,
    `<Name> is also the variable of permission <key>`.
- **Warnings** (append ` (warning)`; fail only with `-strict`):
  - `missing h:desc; the label is "<text>"`
  - `h:perm "<k>" is used by this route only`
  - Against baseline (`the committed file` or `-baseline <git ref>`):
    `key "<k>" is gone since <baseline>: its translations and permission grants are orphaned`,
    `permission "<k>" is gone since <baseline>: its grants are orphaned`,
    `parameter label "route_param.<key>.<p>" is gone since <baseline>: its translations are orphaned`.

## Commands and flags

Install as a Go tool: `go get -tool github.com/epistemer/handlers/cmd/handlers`.
Invoke `go tool handlers <command>`. Exit status: `0` clean, `1` diagnostics
(or warnings with `-strict`), `2` usage.

| Command | Flags | Behavior |
|---|---|---|
| `gen <pkg dir>` | `-out <dir>` (req), `-package <name>`, `-prefix <path>`, `-check`, `-strict`, `-baseline <ref>` | Checks directives and writes `<out>/zz_<pkg>.routes.json`. `-check` fails with `<file> is missing` or `<file> is stale` |
| `openapi <spec>` | `-out <dir>` (req), `-package <name>`, `-key-prefix <key>`, `-check`, `-strict`, `-baseline <ref>` | Checks OpenAPI 3.0.x spec and writes routes JSON |
| `vet <pkgs>` | `-strict`, `-baseline <ref>`, `-hook` | Checks handler packages + all committed `zz_*.routes.json` (each vetted package replaces its matching `source` file) for cross-package key, permission and `ServeMux` collisions. Writes nothing |
| `lsp` | none | JSON-RPC language server over stdin/stdout |

- `vet -hook` reads Claude Code / Gemini CLI JSON (`tool_input.file_path`),
  checks `.go` files inside cwd, prints `{"decision":"block","reason":"handlers vet found faults in the route directives:\n<diagnostics>"}` on errors, and always exits `0`.

## Manifest and generated files

`genx.yaml` (§3.1) and a `//go:build genx` `tools.go` importing
`_ "github.com/epistemer/handlers"` and `_ "github.com/epistemer/i18n"`:

```yaml
version: 1
producers:
  - name: admin-routes
    run:   [go, tool, handlers, gen, -prefix, /admin, -out, internal/admin/adminroutes, ./internal/admin]
    check: [go, tool, handlers, gen, -check, -prefix, /admin, -out, internal/admin/adminroutes, ./internal/admin]
    emits: [internal/admin/adminroutes/zz_adminroutes.routes.json]
  - name: api-routes
    run:   [go, tool, handlers, openapi, -key-prefix, api, -out, internal/api/apiroutes, api/openapi.yaml]
    check: [go, tool, handlers, openapi, -check, -key-prefix, api, -out, internal/api/apiroutes, api/openapi.yaml]
    emits: [internal/api/apiroutes/zz_apiroutes.routes.json]
use:
  - github.com/epistemer/handlers: [routes, labels, permissions]
  - github.com/epistemer/i18n: [labels]
```

Five committed files per surface in `<out>`:
`zz_<pkg>.routes.json`, `zz_<pkg>_routes.go`, `zz_<pkg>.labels.json`,
`zz_<pkg>_i18n.go`, `zz_<pkg>_permissions.go` (omitted when 0 permissions).
`routes` requires `handlers: labels` and `i18n: labels` (for `L10n*` vars).

## Generated Go and runtime API

Every generated routes package exports:
- `const Prefix` (`""` or `"/..."`)
- `var Routes []handlers.Route` (declaration order; do not mutate)
- `func ByKey(key string) (handlers.Route, bool)`
- One typed URL builder per route: `func Order(p0 ids.OrderID) handlers.Link`
  (v0.2.0, amendment A4; escapes segments with `handlers.Segment`,
  `{name...}` with `handlers.Rest`, text types with `handlers.Text`,
  ints/bools with `strconv`). Use as a string: `.URL()`
  (`http.Redirect(w, r, adminroutes.Order(id).URL(), …)`); `%s` works via
  `String()`. Pass it as it is to ui's elements and actions (`ui.Link`).
- Admin (`html`): `type Handler interface` and `func Register(mux *http.ServeMux, h Handler, g handlers.Guard, opts ...handlers.Option)`
- API (`json`): `func Mount(mux *http.ServeMux, g handlers.Guard, spec *openapi3.T, register func(rt *openapi.Router), opts ...handlers.Option)`
- Labels (`i18n`): `L10n<RouteName>`, `L10n<RouteName><Param>`, `L10n(path string) i18n.TKer`
- Permissions (`auth/access`): `Perm<Key>` (`access.Permission(key, text[, auth.Unverified])`)

Runtime (`github.com/epistemer/handlers`):
- `Link` (v0.2.0): `URL()`, `String()`, `Method()`, `Route()` (a copy),
  `With(url.Values)` (replaces the query, keeps a fragment). Comparable.
- `Gate(admit func(Route) bool) func(interface{ URL() string }) bool`: a
  guard's `Admit` check as ui's actions gate. A route link (or a type
  embedding `Link`) asks admit; any other link is admitted. Set it per
  request: `admit, err := guard.Admit(r)`, then
  `actions.Gate.With(ctx, handlers.Gate(admit))`. Gate(nil) panics.
- `Route` (`Key`, `Method`, `Prefix`, `Pattern`, `Surface` (`HTML`/`JSON`),
  `Auth` (`Public`/`Session`/`Permission`), `Permission`, `Label`, `View`,
  `Searchable`, `Entity`, `MaxBody` (bytes; `0` uses the registration's limit,
  else replaces `WithMaxBody`), `Operation`, `Params []Param{Name, Rest, Label}`).
- `From(ctx) (Route, bool)`, `WithRoute(ctx, rt)`.
- `Guard` (`Prepare`, `Check`, `Admit`).
- Options: `WithErrors(ErrorWriter)` (defaults: `WriteText` for HTML,
  `WriteJSON` `{"code": key, "message": text}` for JSON), `WithMaxBody(n)`
  (default 1 MiB; `0` disables), `WithMiddleware(mw...)`.
- `Status(err)` returns `413` for `ErrTooLarge` or `*http.MaxBytesError`,
  else `errors.KindOf(err).Status()`. Writers log statuses `>= 500` via
  `slog.Default()` and never leak `err.Error()`.
- Sentinel errors: `ErrBadParam` (`%s is not valid`, 400), `ErrNotFound`
  (404), `ErrTooLarge` (413), `ErrInvalidRequest` (400), `ErrInvalidField`
  (400), `ErrPanic` (500, "the server failed to answer"; its cause is a
  `*handlers.Panic{Value, Stack}`, for `errors.As`).
- **Recovery** (v0.4.0): every route and catch-all recovers a panic. It is
  logged once (`handlers: panic`, with `route`, `panic`, `stack`), then
  `ErrPanic` goes to the error writer (the default writers don't log it
  again); after a started response the connection is aborted. Headers the
  handler set are dropped, those from before it (outer middleware) kept.
  No opt-out: report panics from the error writer via `*Panic`. Remove an
  app's own recovering wrapper.
- Without a prefix, `Done` registers no catch-all (so `ServeMux` 405 with
  `Allow` remains). `HEAD` hits `GET` routes; `/` registers `<prefix>/{$}`.
- `Mount`, `NewMount`, `Handle`, `HandleHTTP`, `Writer`, `Done`, `Param*`,
  `Segment`, `Rest`, `Text` are for generated code and `openapi`; apps don't
  call them directly.

## `handlers/middleware` (v0.4.0)

The request's lifecycle around handlers, in the root module (errors and
i18n only).

```go
import "github.com/epistemer/handlers/middleware"

fl := middleware.NewFlash() // dev over plain HTTP: middleware.NewFlash(middleware.FlashInsecure())
root.Handle(adminroutes.Prefix+"/", a.Load(auth.Cookie())(fl.Middleware(adminMux)))
srv := &http.Server{Handler: tr.Middleware()(root)} // i18n outside the flash

// A handler that saved:
fl.Add(r.Context(), middleware.LevelSuccess, L10nEventSaved, ev.Name)
return middleware.Redirect(w, r, adminroutes.Event(ev.ID))

// The shell shows the messages that came with this request:
for level, msg := range fl.Messages(ctx) { /* toast(level, msg.T(ctx)) */ }
```

- `Redirect(w, r, to interface{ URL() string })`: htmx → 204 +
  `HX-Redirect`; else 303. Another site's address returns `ErrNotLocal`
  (400, `errors.handlers.not_local`), writing nothing: safe for a return
  address from a query. `HTMX(r)`, `Boosted(r)` read `HX-Request`,
  `HX-Boosted`.
- `ErrorPage(page func(w, r, status int, err error) error)` is an
  `ErrorWriter` for `handlers.WithErrors`: it logs like `WriteText`, calls
  `page` with `handlers.Status(err)`, and falls back to text if `page`
  fails before writing. A 404/500 page is still a returned error
  (`handlers.ErrNotFound`, any error's kind); `page` branches on `HTMX(r)`.
- `Recover(ew)` gives an app's own handlers (outside routes packages) the
  same recovery as routes.
- **The flash.** `Add(ctx, level, msg i18n.Ter, args...)` with `level` one
  of `LevelInfo`, `LevelSuccess`, `LevelWarning`, `LevelError` (plain
  strings). A key travels as key and args and renders in the reader's
  locale; an error or other `Ter` (a `Static`) is rendered when added, as
  text. Args keep their kind (strings, bools, integers including defined
  types, floats, keys). `Add` panics outside `fl.Middleware` (wrap a test
  handler with it) and on another level.
- **When it shows.** `Messages(ctx)` returns the messages the request
  arrived with, never this request's own adds. The first `text/html` page
  (status < 300 or 400–599; content type set, or sniffed) to a request that
  isn't `HEAD` or a non-boosted htmx request delivers them; redirects, 204s,
  JSON, text/plain and downloads carry them on; a 5xx drops what its own
  request added. A page that renders no toast region still uses them up:
  show them in the shell of every page. Wrap the HTML surfaces only.
- The cookie: `__Host-flash` (`FlashName` changes the base), Secure,
  HttpOnly, SameSite=Lax, Path=/, 5 minutes, unsigned base64url JSON;
  `FlashInsecure()` drops Secure and the prefix (tie it to auth's
  `cookie.insecure`).
- **ui** shows flashes as toasts through its own interface,
  `Add(ctx, level string, msg i18n.Ter, args ...any)` and
  `Messages(ctx) iter.Seq2[string, i18n.Ter]`, which `*middleware.Flash`
  satisfies; see the ui skill for how its `Reply` and toasts use it.

## Permissions and `authguard`

```go
guard := authguard.New(a, ac) // github.com/epistemer/handlers/authguard
adminMux := http.NewServeMux()
adminroutes.Register(adminMux, adminH, guard, handlers.WithErrors(admin.ErrorPage))
root.Handle(adminroutes.Prefix+"/", a.Load(auth.Cookie())(adminMux))
```

- Every route is `Permission` (`Verified`) by default, keyed by `//h:perm` or
  its own `//h:key`. Routes sharing `//h:perm` must agree on `unverified`.
- **A permission's text** (v0.4.0): `//h:perm talks.review Review talks`
  (`x-permission: talks.review Review talks`). Routes that give one give
  the same. Without it: the route's text when one route of the declaring
  package uses it, else the readable key. Give shared permissions a text:
  it's what an admin sees when granting.
- **Taking a permission from another routes package** (v0.4.0):
  `//h:perm api.talks.review from apiroutes`, `x-permission: talks.review
  from adminroutes`. `<package>` is the routes package's name (`-package`).
  The word `from` after the key always makes a reference; nothing may follow
  the package (`h:perm: write "h:perm <key> from <package>", with nothing
  after`), and a text can't begin with "from". The taking routes agree with
  the declaration on `unverified`; one grant then opens both surfaces.
  Declaring a key in two packages is still an error. The `permissions`
  generator (inside `go tool genx`) and `vet` check that the package exists
  and declares the key; `gen` checks only its own file, so both can change
  in one edit. Nothing is generated for a reference, and a binary that
  mounts the taking package must link the declaring one, or `Register`
  panics with authguard's `permission "<p>" isn't declared`.
- `authguard.New(a, ac)` panics if `a` or `ac` is nil. `Prepare` fails if
  any `Permission` route's key is not declared in `access.Permissions()`.
- `Check` calls `a.Check(ctx)` for `Session`, `a.Check(ctx, ac.Can(p))` for
  `Permission`, and refuses `Public`. Refusals (`ErrNoSession` 401,
  `ErrNotVerified`/`ErrStale`/`ErrForbidden` 403) flow to the error writer.
- `Admit(r)` reads `auth.From(ctx)` once, memoizes session and per-permission
  checks lazily under a mutex, logs non-refusal errors once at warn level,
  and returns nil error.

## The API side (`handlers openapi` and `openapi.Mount`)

OpenAPI 3.0.x only (`GET`, `POST`, `PUT`, `PATCH`, `DELETE`). Mount prefix
comes from `servers[0].url` path (`""` if no `servers`; trailing `/` or path
variables refused). Path params map to `string`, `int32` (`integer` +
`format: int32`), `int64` (`integer`), `bool` (`boolean`); `x-go-type` on
path parameters is refused in v0.1.

- **Security:** every operation requires `security: []` (`public`) or an HTTP
  `bearer` requirement (permission at `Verified`, or `x-auth: session` for
  `session`). Optional security (`{}` beside schemes) is refused.
- **Extensions:** `x-route-key` (overrides derived `<key-prefix>.<op_id>`),
  `x-permission`, `x-auth: session`, `x-label` (falls back to `summary`,
  then readable `operationId`), `x-max-body` (string size with `body`'s syntax,
  e.g. `"20MiB"`, not on `GET`; errors: `x-max-body must be a string, as "20MiB"`,
  `x-max-body: "<v>" is not a size: write a positive whole number and B, KiB, MiB or GiB, as 20MiB`,
  `x-max-body: "<v>" is larger than a body can be`,
  `x-max-body on a GET route, which has no body`).
- **Errors:** `#/components/schemas/Error` must have string `code` and
  `message`, and every `4XX`/`5XX`/`default` JSON response must use it.
- Run `oapi-codegen` (e.g. via `go:generate`) with `std-http-server: true`,
  `strict-server: true`, `models: true`, `embedded-spec: true` (`GetSpec()`):

```go
spec, err := apigen.GetSpec() // GetSwagger before oapi-codegen v2.8
apiroutes.Mount(apiMux, guard, spec, func(rt *openapi.Router) {
	apigen.HandlerWithOptions(
		apigen.NewStrictHandlerWithOptions(apiSrv, nil, apigen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  rt.RequestError,
			ResponseErrorHandlerFunc: rt.WriteError,
		}),
		apigen.StdHTTPServerOptions{
			BaseURL: apiroutes.Prefix, BaseRouter: rt, ErrorHandlerFunc: rt.RequestError,
		})
})
root.Handle(apiroutes.Prefix+"/", a.Load(auth.Bearer())(apiMux))
```

- `openapi.Router` validates requests against the operation via
  `kin-openapi` (`AuthenticationFunc: NoopAuthenticationFunc`, JSON body put
  back for handler, `multipart/form-data` streamed via `ExcludeRequestBody`),
  strips rejected input values from causes, and maps failures to
  `ErrTooLarge`, `ErrInvalidField` (parameter name or JSON pointer), or
  `ErrInvalidRequest`.

## Search

```go
table := handlers.NewTable(adminroutes.Routes, apiroutes.Routes) // panics on duplicate key
hits, err := table.Search(r, guard, tr, handlers.Query{
	Text:  q,
	Only:  func(rt handlers.Route) bool { return rt.Searchable },
	Limit: 20,
})
```

`Table.Search` calls `g.Admit(r)` once (or admits `Public` only if `g == nil`),
then runs one `tr.Search` over prefix `route.` in `i18n.Code(r.Context())`
(falling back to `tr.Default()`), matching `Text` case-insensitively against
either translated or source text, returning `Hit{Route, Text}` in key order.

## Testing

- `handlerstest.Guard{Session: true, Granted: map[string]bool{"orders.manage": true}}`
  is a fake `handlers.Guard` returning `handlerstest.ErrNoSession` (401) or
  `handlerstest.ErrForbidden` (403).
- `handlerstest.AssertPublic(t, adminroutes.Routes, "sign_in", ...)` fails
  if public routes drift.
- `genxtest.AssertGenerated(t)` (plus a test running `go tool handlers vet -strict ./...`)
  ensures generated files stay current.
- For end-to-end tests, combine `authtest.New(t)`, `i18ntest.New(t, ...)`,
  `access.New(db)`, `authguard.New(kit.Auth, ac)`, and `httptest.NewTLSServer`
  with a cookie jar (align `kit.Clock` to wall time for cookie jars; see
  `examples/shop/main_test.go`).

## Editor and agent tools

- `go tool handlers lsp`: syntax diagnostics on open/change, syntax + type
  pass on save, completion (`//h:`, values, handler params, `entity`),
  hover, `entity` go-to-definition, UTF-8/UTF-16 position negotiation.
- `dev/vscode`: VS Code client for `handlers lsp` + TextMate grammar.
- `dev/claude-plugin`: `PostToolUse` hook (`handlers vet -hook`) + this skill
  (`handlers@epistemer-handlers`).
- `dev/gemini-extension`: `AfterTool` hook (`handlers vet -hook`) + `GEMINI.md`.

## Gotchas

- **Reserved names:** handler methods / `operationId`s cannot be `Handler`,
  `Register`, `Mount`, `Routes`, `ByKey`, `Prefix`, `L10n`, or collide with
  any generated `L10n*` or `Perm*` variable.
- **One handler type per package:** all `//h:` methods in a Go package must
  be on the same receiver type.
- **Declare a permission in one routes package:** declaring the same key in
  two `routes/v1` files fails `permissions` generation and `handlers vet`
  (`...; declare it in one and take it from there with "from <package>"`).
  Take it elsewhere with `from`.
- **Generated code is a separate package:** never pass `-out` to the handler
  package itself, and put custom path-parameter types in a separate package
  (e.g. `internal/ids`) so both packages can import them without a cycle.
- **Keys and parameter names are permanent:** translations (`route.<key>`,
  `route_param.<key>.<param>`) and `auth/access` grants are keyed by them.
  Keep `-baseline origin/main` in CI so removals warn before merge.
- **Upgrading from v0.1.0:** require v0.2.0, run `go tool genx` first (the
  handler package still type-checks against the old string builders), then
  add `.URL()` where the compiler asks.
- **Write the handler, generate, then link.** Run `go tool genx` (or
  `handlers gen`) after adding a route and before using its URL builder or
  its `L10n*`/`Perm*` variables. A package that links to a builder that
  doesn't exist yet doesn't type-check, so gen refuses it and names the
  builder (`adminroutes.EventsDelete doesn't exist yet: generate the route
  before linking to it`): remove the link, generate, link again. No stub
  tool is needed.
- **Surface isolation:** mount admin (`auth.Cookie()`) and API
  (`auth.Bearer()`) on separate `http.ServeMux` instances so cookies cannot
  reach API routes or bearer tokens reach admin routes.

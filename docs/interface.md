# handlers — Interface

> Status: **locked on 2026-11-18**, by Claude on epistemer's instruction to
> freeze it. A change to anything here is an amendment (§17); one that
> changes what an app writes or gets back is epistemer's to decide. The
> choices Claude made in it were confirmed by epistemer the same day. Reviewed before the lock:
> [review 0005](reviews/0005-interface.md).
>
> This document says what an app writes and what it gets back. The design
> and its reasons are [ADR 0001](adr/0001-routes-domain.md) and
> [brief.md](brief.md); the spikes are ADRs 0002–0004.

## 1. Modules and packages

| Module | Package | What | Depends on |
|---|---|---|---|
| `github.com/epistemer/handlers` | `handlers` | The runtime (§11) and `genx.meta.yaml` with the generators `routes`, `labels`, `permissions` (§9) | errors, i18n |
| | `handlers/handlerstest` | Test helpers (§11.6) | |
| `github.com/epistemer/handlers/authguard` | `authguard` | A `Guard` over auth (§11.4) | auth |
| `github.com/epistemer/handlers/openapi` | `openapi` | The adapter for oapi-codegen's server (§11.5) | kin-openapi |
| `github.com/epistemer/handlers/cmd` | `cmd/handlers` | The command: `gen`, `openapi`, `vet`, `lsp` (§6) | golang.org/x/tools, kin-openapi |

An app with only admin pages needs the root module, `authguard` and the
command. The `openapi` module is for an API.

Tools in `dev/` (§15) are versioned with the repository, not as Go modules.

## 2. Decisions

The brief's table (D1–D19) and the spikes, as they stand at the freeze.

| # | Decision | Source |
|---|---|---|
| D1 | Module and package `handlers`; label keys `route.<key>`, `route_param.<key>.<param>` | epistemer |
| D2 | Admin routes are comment directives on handler methods, checked like code | epistemer |
| D3 | The API is spec-first: one OpenAPI 3.0 file | epistemer; ADR 0003 |
| D4 | handlers declares `auth/access` permissions for routes and guards them | epistemer |
| D5 | Editor and agent tools live in `dev/` | epistemer |
| D6 | Two producers write `routes/v1`; three trivial generators read it | ADR 0001 |
| D7 | `routes/v1` is owned here until a second package uses it | ADR 0001 |
| D8 | `net/http` `ServeMux` only | brief |
| D9 | A handler is `func (h *H) X(w http.ResponseWriter, r *http.Request, <path params>) error` | brief |
| D10 | No route meta in i18n: a generated route table instead | ADR 0001 |
| D11 | Auth: `public`, `session`, `unverified`, default the route's permission at `Verified` | epistemer |
| D12 | Fail closed: route in context before the guard; registration panics without a guard or with an undeclared permission | brief |
| D13 | The API's server is oapi-codegen's strict `std-http` server through handlers' adapter | ADR 0003 |
| D14 | Handlers return errors; handlers writes them from `errors.Public` and `errors.KindOf` | brief |
| D15 | The core depends on errors and i18n only; auth, kin-openapi and x/tools are in nested modules | brief; this document |
| D16 | Route keys are permanent; a removed key is a warning | epistemer |
| D17 | Rendering, menus and CRUD tables are out of scope | brief |
| D18 | `//h:param <name> label <text>` replaces `//p[<name>]:label` | epistemer |
| D19 | The VS Code extension is a client of `handlers lsp`; the Claude Code plugin ships the hook, not a language server | epistemer; ADR 0004 |

## 3. What an app writes

### 3.1 The manifest

```yaml
# genx.yaml
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

- `routes` needs `labels` and i18n's `labels`: the generated code refers to
  the label variables they generate in the same package.
- `permissions` needs auth in the app's module graph. An app without auth
  leaves it out, and then can't mount routes other than `public` ones
  without its own `Guard`.
- The command is a `tool` in the app's `go.mod`
  (`go get -tool github.com/epistemer/handlers/cmd/handlers`).
- The app keeps a `//go:build genx` file that imports
  `github.com/epistemer/handlers` blank, as genx requires.

### 3.2 Files

| File | Written by | Committed |
|---|---|---|
| `<out>/zz_<package>.routes.json` | `handlers gen` or `handlers openapi` | yes |
| `<out>/zz_<package>_routes.go` | generator `routes` | yes |
| `<out>/zz_<package>.labels.json` | generator `labels` | yes |
| `<out>/zz_<package>_i18n.go` | i18n's generator `labels` | yes |
| `<out>/zz_<package>_permissions.go` | generator `permissions` | yes |

`<out>` is the producer's `-out`; `<package>` is `-package`, default the
last element of `-out`. The generated package is never the handler package
itself (§8).

### 3.3 Wiring

```go
guard := authguard.New(a, ac)

admin := http.NewServeMux()
adminroutes.Register(admin, &adminui.H{...}, guard, handlers.WithErrors(adminui.ErrorPage))

api := http.NewServeMux()
spec, _ := apigen.GetSpec() // oapi-codegen's embedded spec (GetSwagger before v2.8)
apiroutes.Mount(api, guard, spec, func(rt *openapi.Router) {
	apigen.HandlerWithOptions(
		apigen.NewStrictHandlerWithOptions(apiImpl, nil, apigen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  rt.RequestError,
			ResponseErrorHandlerFunc: rt.WriteError,
		}),
		apigen.StdHTTPServerOptions{BaseURL: apiroutes.Prefix, BaseRouter: rt, ErrorHandlerFunc: rt.RequestError})
})

root := http.NewServeMux()
root.Handle(adminroutes.Prefix+"/", a.Load(auth.Cookie())(admin))
root.Handle(apiroutes.Prefix+"/", a.Load(auth.Bearer())(api))
srv := &http.Server{Handler: tr.Middleware()(root)}
```

- Each surface gets its own mux and its own `a.Load`, so the API accepts
  only bearer tokens and admin pages only the cookie. Both packages have a
  prefix here; a package without one is mounted at `/` on its own mux.
- The app wraps the muxes with i18n's middleware and auth's `Load`, as those
  packages require. handlers registers only its routes and, under a
  prefix, one catch-all (§12.4).

## 4. Directives

### 4.1 Form

A directive is a comment line `//h:<name>` or `//h:<name> <value>`: no space
after `//`, a lower-case name, one space before the value. The value runs
to the end of the line, without leading or trailing spaces. gofmt leaves
this form alone and godoc hides it.

A handler's directives are its doc comment (they may share it with ordinary
comment lines). A handler is an exported method whose first two parameters
are `http.ResponseWriter` and `*http.Request`, whose other parameters are
its path parameters, in any grouping, and which returns `error`. All the
handlers of a package are methods of one type.

```go
//h:key categories.profile
//h:method GET
//h:path /categories/{id}
//h:desc Category detail
//h:param id label Category ID
//h:entity categories.search
func (h *H) Category(w http.ResponseWriter, r *http.Request, id int64) error {
```

### 4.2 Directives and values

| Directive | Value | Default |
|---|---|---|
| `key` | dot-separated segments, each `[a-z0-9][A-Za-z0-9_]*` | required |
| `method` | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, any case | required |
| `path` | starts with `/`; doesn't end with `/` unless it is `/`; `ServeMux` wildcards `{name}` and, last, `{name...}`; no `{$}` | required |
| `desc` | text, not empty | the key made readable (§13.3), with a warning |
| `auth` | `public`, `session`, `unverified` | the route's permission at `Verified` |
| `perm` | a key, as `key` | the route's key |
| `view` | none, or `true` | off |
| `search` | none, or `true` | off |
| `entity` | another route's key, in the same package, not its own | none |
| `param` | `<name> label <text>`, `<name>` a parameter of the handler; once per parameter | the parameter's name made readable |

- `unverified` declares the route's permission at `auth.Unverified`.
- `perm` is only for routes whose `auth` is the default or `unverified`.
  Routes that share one must agree on `unverified`.
- Parameter types: `string`, `bool`, the integer types other than `uintptr`, a type whose
  underlying type is one of those, or a type `T` with `*T` implementing
  `encoding.TextUnmarshaler` and `T` implementing `encoding.TextMarshaler`;
  when a type is both, the text methods win. A `{name...}` parameter is a
  string type. A parameter type declared in the
  handler's own package is refused (§8).

## 5. Diagnostics

### 5.1 Form

Every diagnostic is `<file>:<line>:<col>: <message>`, the position of the
directive line or of the handler's `func` keyword; on the command line,
warnings end with ` (warning)`. Messages are stable: tests compare them word
for word, except the part a message quotes from Go (`ServeMux`'s reason, a
type error), which tests match by its fixed prefix. A run with an error writes nothing; warnings don't stop it unless
`-strict` is given.

### 5.2 Errors

| Case | Message |
|---|---|
| A name that isn't a directive | `unknown directive h:<name>` |
| A near-miss: `//`, optional spaces, `h` or `H`, optional spaces and `:`; or `//`, optional spaces and `p[` (prose such as `// h key …` or `// Handler …` never matches) | `not a directive: write "<fixed>"` (the line as a directive, `//p[id]:label X` → `//h:param id label X`) |
| A directive twice on one handler | `h:<name> given twice`; for `param`, `h:param <name> given twice` |
| `method` | `h:method: "<v>" is not GET, POST, PUT, PATCH or DELETE` |
| `auth` | `h:auth: "<v>" is not public, session or unverified` |
| `view`, `search` | `h:<name> takes no value or true, not "<v>"` |
| `desc` empty | `h:desc is empty` |
| `key`, `perm`, `entity` | `h:<name>: "<v>" is not a key` |
| `path` | `h:path: "<v>" must start with "/"` · `h:path: "<v>" must not end with "/"` · `h:path: "<v>": <ServeMux's reason>` |
| `param` | `h:param: write "h:param <name> label <text>"` · `h:param: no parameter <name>` |
| `perm` with `public` or `session` | `h:perm needs a permission route, not auth <auth>` |
| Routes sharing a permission disagree on `unverified` | `permission "<k>": <key a> is unverified and <key b> isn't` |
| `entity` naming its own route | `h:entity names the route itself` |
| Directives on two types | `directives on <A> and <B>: one handler type per package` |
| A missing directive | `missing h:<name>` (key, method, path) |
| Directives outside a doc comment | `directives must be the doc comment of a handler method` |
| Not a handler | `<Name> isn't a handler: <reason>`, reason one of `not a method`, `not exported`, `the first parameters must be http.ResponseWriter and *http.Request`, `it must return error` |
| A duplicate key | `key "<k>" also at <file>:<line>` |
| A duplicate method and pattern | `<METHOD> <prefix><path> also at <file>:<line>` |
| A conflict `ServeMux` refuses | `<METHOD> <prefix><path> conflicts with <METHOD> <prefix><path> at <file>:<line>` |
| A wildcard without a parameter | `wildcard {<name>} has no parameter` |
| A parameter without a wildcard | `parameter <name> has no wildcard in <path>` |
| `{name...}` with a non-string parameter | `parameter <name>: a {<name>...} wildcard needs a string type` |
| A type the binder can't parse | `parameter <name>: type <T> can't be read from a path` |
| A type from the handler's package | `parameter <name>: type <T> is declared in this package; declare it in another so the generated package can import it` |
| `entity` unknown | `h:entity: no route with key "<k>"` |
| Two keys, one Go name | `keys "<a>" and "<b>" both give the Go name <Name>` |
| A reserved or taken Go name | `<Name> is a name the generated package declares` · `<Name> is also the label variable of <key>` · `<Name> is also the variable of permission <key>` |
| The package doesn't type-check | `the package doesn't type-check: <first error>` |

### 5.3 Warnings

| Case | Message |
|---|---|
| No `desc` | `missing h:desc; the label is "<text>"` |
| A route key gone since the baseline | `key "<k>" is gone since <baseline>: its translations and permission grants are orphaned` |
| A permission gone | `permission "<k>" is gone since <baseline>: its grants are orphaned` |
| A parameter gone | `parameter label "route_param.<key>.<p>" is gone since <baseline>: its translations are orphaned` |
| A `perm` only this route uses | `h:perm "<k>" is used by this route only` |

`<baseline>` is `the committed file`, or the git ref given with
`-baseline`. A routes file that doesn't exist at the baseline (a new
package) gives no warnings.

## 6. Commands

`go tool handlers <command>`. Exit status 0, 1 on diagnostics (errors, or
warnings under `-strict`), 2 on usage.

| Command | Flags | Does |
|---|---|---|
| `gen <package dir>` | `-out <dir>` (required), `-package <name>`, `-prefix <path>` (default `""`), `-check`, `-strict`, `-baseline <git ref>` | Checks the handler package (§5) and writes `<out>/zz_<package>.routes.json`. `-check` writes nothing, and fails with `<file> is missing` or `<file> is stale` |
| `openapi <spec>` | `-out`, `-package`, `-key-prefix <key>` (default none), `-check`, `-strict`, `-baseline` | Checks the spec (§7) and writes the routes file |
| `vet <package dirs or patterns>` | `-strict`, `-baseline`, `-hook` | Checks handler packages, and every committed `zz_*.routes.json` of the module with them: a package takes the prefix of the routes file whose `source` is its directory, and stands in for that file, so keys, permission keys and patterns are compared across packages without comparing a package with itself. Writes nothing |
| `lsp` | none | The language server (§15) on stdin and stdout |

- The baseline for removed keys is the routes file on disk before `gen`
  overwrites it; with `-baseline <ref>`, it is that file at the git ref (CI
  passes the base branch).
- `vet -hook` reads a Claude Code `PostToolUse` payload on stdin, checks the
  package of `tool_input.file_path` when it is a `.go` file inside the
  working directory, and, if it finds errors, prints
  `{"decision":"block","reason":"handlers vet found faults in the route directives:\n<diagnostics>"}`.
  It prints nothing otherwise and always exits 0.

## 7. The OpenAPI producer

### 7.1 What it reads

An OpenAPI 3.0.x document. The mount prefix is the path of `servers[0].url`
(`https://host/api/v1` and `/api/v1` give `/api/v1`; no `servers`, `""`).
Operations are `GET`, `POST`, `PUT`, `PATCH` and `DELETE`.

Path parameters map to Go as oapi-codegen maps them:

| Schema | `type` | `parse` |
|---|---|---|
| `string` (any `format`) | `string` | `string` |
| `integer` with `format: int32` | `int32` | `int32` |
| `integer`, other or no `format` | `int64` | `int64` |
| `boolean` | `bool` | `bool` |

Anything else on a path parameter, `x-go-type` included, is an error in
v0.1.

### 7.2 Errors

Messages are `<spec>: <location>: <message>`, the location as
`<METHOD> <path>` or a JSON pointer.

| Case | Message |
|---|---|
| Not 3.0.x | `openapi: "<v>" is not 3.0.x` |
| `servers[0].url` unusable | `servers[0].url: "<v>" is not a URL` · `servers[0].url: "<v>": the path must not end with "/"` · `servers[0].url: "<v>": the path must not hold a variable` |
| Another method | `<M> <p>: method <M> isn't supported` |
| No `operationId`, or a duplicate | `<M> <p>: no operationId` · `<M> <p>: operationId "<id>" also at <M> <p>` |
| An `operationId` that isn't a Go identifier | `<M> <p>: operationId "<id>" is not a Go identifier` |
| Two operations, one Go name; a reserved name | `<M> <p>: operationId "<id>" gives the Go name <Name>, also from <M> <p>` · `<M> <p>: <Name> is a name the generated package declares` |
| No security, own or top-level | `<M> <p>: no security; write security: [] for a public operation` |
| Optional security (an empty requirement beside others) | `<M> <p>: optional security isn't supported` |
| An unknown scheme | `<M> <p>: security names undeclared scheme "<s>"` |
| A scheme other than HTTP bearer | `<M> <p>: scheme "<s>" isn't http bearer` |
| `x-permission` or `x-auth` on a public operation | `<M> <p>: x-permission on a public operation` · `<M> <p>: x-auth on a public operation` |
| `x-auth` with `x-permission` | `<M> <p>: x-auth: session takes no x-permission` |
| `x-auth` other than `session` | `<M> <p>: x-auth must be session` |
| `x-permission`, `x-route-key` or `-key-prefix` not a key | `<M> <p>: x-permission "<v>" is not a key` · `<M> <p>: x-route-key "<v>" is not a key` · `-key-prefix "<v>" is not a key` |
| Two operations, one key | `<M> <p>: key "<k>" also from operationId "<a>"` |
| An empty `x-label` | `<M> <p>: x-label is empty` |
| An operation name equal to a label or permission variable | as §5.2 |
| An error response without the `Error` schema | `<M> <p>: response <code> doesn't use #/components/schemas/Error` (every `4XX`, `5XX` and `default` response with a JSON body) |
| `Error` missing or wrong | `components/schemas/Error: needs string properties code and message` |
| A path the mux can't route | `<M> <p>: path parameter "<n>" is not a Go identifier` · `<M> <p>: parameter {<n>} doesn't fill its segment` · `<M> <p>: parameter "<n>" sets allowReserved` |
| A path parameter's schema | `<M> <p>: path parameter "<n>": <schema> isn't supported` |

Warnings: removed keys and permissions, as §5.3, and
`<M> <p>: x-permission "<k>" is used by this operation only`.

Parameters declared on a path item apply to its operations, and `$ref`s
are resolved before any check. `openapi -check` fails as `gen -check` does.

### 7.3 Extensions

| Extension | On | Means |
|---|---|---|
| `x-route-key` | operation | The whole route key (no `-key-prefix`), pinned across an `operationId` rename |
| `x-permission` | operation | The permission's whole key; default the route key |
| `x-auth: session` | operation | A signed-in session without a permission |
| `x-label` | operation, path parameter | The label text; default the operation's `summary`, else the `operationId` made readable; a parameter's name made readable |

`x-go-type` and `x-go-type-import` are oapi-codegen's and pass through,
except on path parameters (§7.1).

### 7.4 Routes it writes

- Key: `x-route-key`, else the `operationId` split at case changes (a run of
  capitals is one word: `getUserID` → `get_user_id`) and at `-` and `_`,
  lower-cased and joined with `_`, then prefixed with `<key-prefix>.` when
  given.
- Auth: `security: []` is `public`; a bearer requirement is the route's
  permission (`x-permission` or the key), at `Verified`; with
  `x-auth: session`, `session`.
- Name: the `operationId` with its first letter upper-cased.
- Pattern: the path, with `{param}` kept; `operation` is the `operationId`;
  `source` is the spec's path.

## 8. `routes/v1`

The domain file both producers write: shape `schema/routes.v1.json`, rules
`internal/routesv1/rules.md` (part of this contract), conformance files
`testdata/routes/`. Producers check every file they write with
`routesv1.Check` and fail rather than write one it refuses.

Go names, resolved by the producer:

| Name | Rule |
|---|---|
| Route `name` | The handler method's name; on the API, the `operationId` with its first letter upper-cased |
| Route `var` | `L10n` + name |
| Parameter `go` | The handler's parameter name; on the API, the wildcard's name |
| Parameter `var` | `L10n` + route name + parameter `go` with its first letter upper-cased |
| Permission `var` | `Perm` + the key split at `.`, `_` and `-`, each part's first letter upper-cased (`pricingPlans.fees` → `PermPricingPlansFees`) |
| `type`, `import`, `alias` | The parameter's type as the generated package spells it (`shop.OrderID`), its package's import path (`""` for predeclared types) and the qualifier it is imported as. Aliases are the package's name, numbered on a collision (`shop2`), and never a name under which the generated file imports its own packages (`handlers`, `http`, `strconv`, `url`, `access`, `auth`, `openapi`, `openapi3`) |
| `parse` | `text` for a text type; else the underlying type: `string`, `bool`, `int`, `int8` … `int64`, `uint`, `uint8` … `uint64` |
| `source` | The handler package's directory, or the spec's path, relative to the module root |

## 9. Generators

In `github.com/epistemer/handlers`'s `genx.meta.yaml`. No parameters.

| Generator | Needs | Writes |
|---|---|---|
| `routes` | `routes/v1` | `<dir>/zz_<package>_routes.go` per routes file (§10) |
| `labels` | `routes/v1` | emits `labels/v1`: `<dir>/zz_<package>.labels.json` (§13) |
| `permissions` | `routes/v1` | `<dir>/zz_<package>_permissions.go` per routes file that declares a permission. A permission belongs to one routes package: a key in two routes files stops it with `permission "<k>" is in <dir a> and <dir b>; a permission belongs to one routes package` (sharing across packages is §16's) |

## 10. Generated Go

### 10.1 Every routes package

```go
// Code generated by handlers; DO NOT EDIT.
package adminroutes

const Prefix = "/admin"

// Routes is the package's route table, in declaration order.
var Routes = []handlers.Route{ /* one per route */ }

func ByKey(key string) (handlers.Route, bool)

// One URL builder per route, named as the route, taking its parameters in
// order: Prefix + the path with each parameter written in.
func Category(id int64) string // "/admin/categories/42"
```

URL builders write strings with `handlers.Segment` (a `{name...}`
parameter with `handlers.Rest`), integers and booleans with `strconv`, and
a text type with `handlers.Text`.

Generated code names parameters `p0`, `p1`, … and imports every package
under an explicit alias, so no parameter or type name can shadow what it
uses. `Routes` is the table `Register` and `Mount` read: an app must not
change it.

### 10.2 Admin pages (`surface: html`)

```go
type Handler interface {
	Category(w http.ResponseWriter, r *http.Request, id int64) error
	// one method per route, as the handler declares it
}

// Register registers every route on mux (§12). It panics when a route
// isn't public and g is nil, or when g refuses a route.
func Register(mux *http.ServeMux, h Handler, g handlers.Guard, opts ...handlers.Option)
```

### 10.3 The API (`surface: json`)

```go
// Mount registers the API's routes on mux through the router it passes to
// register, which hands it to oapi-codegen's HandlerWithOptions. When
// register returns, Mount panics if a route of the table wasn't registered.
func Mount(mux *http.ServeMux, g handlers.Guard, spec *openapi3.T, register func(rt *openapi.Router), opts ...handlers.Option)
```

### 10.4 Permissions

```go
// Code generated by handlers; DO NOT EDIT.
package adminroutes

var (
	PermCategoriesManage = access.Permission("categories.manage", "Manage categories")
	PermOrdersRefund     = access.Permission("orders.refund", "Refund orders", auth.Unverified)
)
```

## 11. Runtime API

### 11.1 Routes

```go
package handlers // import "github.com/epistemer/handlers"

type Surface uint8

const (
	HTML Surface = iota + 1
	JSON
)

type Auth uint8

const (
	Public Auth = iota + 1
	Session
	Permission
)

func (s Surface) String() string // "html", "json"
func (a Auth) String() string    // "public", "session", "permission"

type Route struct {
	Key        string
	Method     string    // GET, POST, PUT, PATCH, DELETE
	Prefix     string    // the package's mount prefix
	Pattern    string    // the path as declared, without the prefix
	Surface    Surface
	Auth       Auth
	Permission string    // the permission's key when Auth is Permission
	Label      i18n.TKer // route.<key>
	View       bool
	Searchable bool
	Entity     string    // a route key, or ""
	Operation  string    // the operationId on the JSON surface
	Params     []Param
}

type Param struct {
	Name  string    // the wildcard's name
	Rest  bool      // {name...}
	Label i18n.TKer // route_param.<key>.<name>
}

// From returns the route of the request ctx belongs to.
func From(ctx context.Context) (Route, bool)
func WithRoute(ctx context.Context, rt Route) context.Context
```

### 11.2 Guards, options and errors

```go
type Guard interface {
	// Prepare is called once per route that isn't public, at registration;
	// an error makes registration panic.
	Prepare(rt Route) error
	// Check admits a request to a route that isn't public, or returns the
	// error to answer with.
	Check(r *http.Request, rt Route) error
	// Admit returns a check for many routes, with the request's session
	// read once (§14). A request without a session, or without a
	// permission, gets a check that refuses those routes; the error is for
	// a failure to read the session or the permissions.
	Admit(r *http.Request) (func(Route) bool, error)
}

type Option func(*options)

func WithErrors(w ErrorWriter) Option                            // default WriteText (HTML), WriteJSON (JSON)
func WithMaxBody(n int64) Option                                 // default 1 MiB; 0 means no limit
func WithMiddleware(mw ...func(http.Handler) http.Handler) Option // after the guard, before binding

type ErrorWriter func(w http.ResponseWriter, r *http.Request, err error)

func WriteText(w http.ResponseWriter, r *http.Request, err error) // text/plain: errors.T in the request's locale
func WriteJSON(w http.ResponseWriter, r *http.Request, err error) // {"code": key, "message": text}
func Status(err error) int                                        // §12.3

var (
	ErrBadParam       = errors.Key1[i18n.TKer]("errors.handlers.bad_param", "%s is not valid", errors.Invalid)
	ErrNotFound       = errors.Key("errors.handlers.not_found", "not found", errors.NotFound)
	ErrTooLarge       = errors.Key("errors.handlers.too_large", "the request is too large", errors.Invalid)
	ErrInvalidRequest = errors.Key("errors.handlers.invalid_request", "the request is not valid", errors.Invalid)
	ErrInvalidField   = errors.Key1[string]("errors.handlers.invalid_field", "the request is not valid at %s", errors.Invalid)
)
```

- `ErrBadParam`'s argument is the parameter's label, which errors renders
  in the request's locale.
- `ErrInvalidField`'s is the failing parameter's name or the body's JSON
  pointer, never a value. `ErrInvalidRequest` is for a failure without a
  location, such as malformed JSON.
- `WriteText` and `WriteJSON` are the defaults; `WriteJSON` is also the
  hook for code outside a route.

### 11.3 For generated code

```go
type Mount struct{ /* unexported */ }

func NewMount(mux *http.ServeMux, routes []Route, g Guard, opts ...Option) *Mount
func (m *Mount) Handle(i int, h func(w http.ResponseWriter, r *http.Request) error)
// HandleHTTP registers routes[i] like Handle, with h as the handler and validate (if not
// nil) run after the middleware: the API's step 5. h writes its own response.
func (m *Mount) HandleHTTP(i int, h http.Handler, validate func(*http.Request) error)
// Writer returns the error writer the options chose.
func (m *Mount) Writer() ErrorWriter
func (m *Mount) Done() // registers the catch-all; panics if a route wasn't handled

func ParamString(r *http.Request, p Param) string
func ParamInt(r *http.Request, p Param, bits int) (int64, error)
func ParamUint(r *http.Request, p Param, bits int) (uint64, error)
func ParamBool(r *http.Request, p Param) (bool, error)
func ParamText(r *http.Request, p Param, v encoding.TextUnmarshaler) error

func Segment(s string) string             // url.PathEscape
func Rest(s string) string                // each segment escaped, slashes kept
func Text(v encoding.TextMarshaler) string // panics if MarshalText fails
```

Apps don't call these; they are exported because the generated package
does.

### 11.4 `authguard`

```go
package authguard // import "github.com/epistemer/handlers/authguard"

// New returns a Guard over auth: session routes need a verified session
// (a.Check), permission routes a.Check with ac.Can of the declared
// permission. Prepare refuses a permission that isn't declared:
// `authguard: route "<key>": permission "<p>" isn't declared`.
func New(a *auth.Auth, ac *access.Access) handlers.Guard
```

Refusals are auth's errors (`ErrNoSession` 401; `ErrNotVerified`,
`ErrStale`, `ErrForbidden` 403) and reach the route's error writer.

`Admit` reads the session once (`auth.From`), and checks each distinct
permission at most once per call with `a.Check(ctx, ac.Can(p))`.

### 11.5 `openapi`

```go
package openapi // import "github.com/epistemer/handlers/openapi"

// Router implements oapi-codegen's ServeMux interface.
type Router struct{ /* unexported */ }

// Mount calls register with a Router over mux, then panics if a route of
// routes wasn't registered. It panics on a nil mux, spec or register.
func Mount(mux *http.ServeMux, routes []handlers.Route, g handlers.Guard, spec *openapi3.T, register func(*Router), opts ...handlers.Option)

func (r *Router) HandleFunc(pattern string, h func(http.ResponseWriter, *http.Request))
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request)

// RequestError and WriteError are oapi-codegen's error hooks: they write
// through the error writer of Mount's options. RequestError writes
// ErrTooLarge for a body over the limit, else ErrInvalidRequest, and logs
// the cause with slog at debug level; WriteError writes the handler's error.
func (r *Router) RequestError(w http.ResponseWriter, req *http.Request, err error)
func (r *Router) WriteError(w http.ResponseWriter, req *http.Request, err error)
```

- `HandleFunc` registers `<prefix>/` as `<prefix>/{$}`, and panics, as
  `handlers/openapi: <pattern>: <reason>`, on a pattern without a route, a
  path the spec doesn't have, or an operation missing from its path item.
- Validation finds the operation from the route, never from the spec's
  `servers`, and leaves authentication to the guard.

### 11.6 `handlerstest`

```go
package handlerstest // import "github.com/epistemer/handlers/handlerstest"

// Guard is a fake: a session when Session is true, and the permissions in
// Granted. Prepare accepts every route; Check refuses with ErrNoSession or
// ErrForbidden.
type Guard struct {
	Session bool
	Granted map[string]bool
}

var (
	ErrNoSession = errors.Key("errors.handlerstest.no_session", "no session", errors.Unauthenticated)
	ErrForbidden = errors.Key("errors.handlerstest.forbidden", "forbidden", errors.Forbidden)
)

// AssertPublic fails t unless the public routes of routes are exactly keys.
func AssertPublic(t testing.TB, routes []handlers.Route, keys ...string)
```

### 11.7 Search

```go
type Table struct{ /* unexported */ }

func NewTable(routes ...[]Route) *Table // panics on a key in two tables
func (t *Table) ByKey(key string) (Route, bool)
func (t *Table) Routes() []Route

type Query struct {
	Text  string           // matched, ignoring case, against the text in the request's locale and the source text
	Only  func(Route) bool // optional, e.g. searchable pages
	Limit int              // 0 means no limit
}

type Hit struct {
	Route Route
	Text  string // the label in the request's locale
}

func (t *Table) Search(r *http.Request, g Guard, tr *i18n.Translator, q Query) ([]Hit, error)
```

## 12. Behaviour

### 12.1 The chain

Every registered route answers in this order:

1. The body is limited (`WithMaxBody`).
2. The route goes into the context (`From`).
3. The guard checks it, unless it is public.
4. `WithMiddleware`'s middleware runs.
5. On the API, kin-openapi validates the request against the operation: path
   and query parameters, and a JSON body, which it reads and puts back for
   the handler; a multipart body isn't read. A failure with a location is
   `ErrInvalidField`, one without (malformed JSON, a wrong content type) is
   `ErrInvalidRequest`, and a body over the limit is `ErrTooLarge`.
6. Parameters are bound; a bad one is `ErrBadParam` (admin) or, through
   `RequestError`, `ErrInvalidRequest` (API; step 5 has already checked
   them, so this is rare).
7. The handler runs; its error goes to the error writer.

### 12.2 Fail closed

- `Register` and `Mount` panic on a nil mux or handler, when a route isn't
  public and the guard is nil, and when `Guard.Prepare` refuses a route.
- Both panic if a route of the table wasn't registered (`Register` through
  `Mount.Done`; `openapi.Mount` when `register` returns).
- An unknown `Surface` or `Auth` value panics wherever it is met.

### 12.3 Statuses

`Status(err)` is 413 when `errors.Is(err, ErrTooLarge)` or `errors.As`
finds an `*http.MaxBytesError`, else `errors.KindOf(err).Status()`. The
writers log an error whose status is 500 or more with `log/slog`'s default
logger, which an app can replace. People never see `Error()` text.

### 12.4 Not found

Under a prefix, `Done` registers `<prefix>/` on the mux: an unknown path
there, or a known path with another method, is `ErrNotFound` (404, no
`Allow`, so a caller learns nothing about routes it can't use), through the
package's error writer. A package without a prefix registers no catch-all
and leaves the mux's own answers, `405` with `Allow` included: give a
package a prefix to keep them in its format. One package per prefix: a
second panics in `ServeMux`. `HEAD` reaches a `GET` route, as `ServeMux`
does.

### 12.5 Patterns

A route registers `<METHOD> <prefix><path>`. The path `/` registers
`<prefix>/{$}`, so it doesn't match the subtree.

## 13. Labels and permissions

### 13.1 Labels

The `labels` generator writes, per routes file, a `labels/v1` file with
producer `handlers`, the routes package, and:

| Key | Text | `var` | `path` |
|---|---|---|---|
| `route.<key>` | the route's text | route `var` | `route.<key>` |
| `route_param.<key>.<param>` | the parameter's label | parameter `var` | `route_param.<key>.<param>` |

No `meta`.

### 13.2 Permissions

`access.Permission(<key>, <text>[, auth.Unverified])` per permission of the
routes file: the key `permission.<key>` in i18n, registered by auth. A
permission's text is its route's text when one route uses it, and its key
made readable when several do.

### 13.3 Readable text

A name made readable: split at `.`, `_`, `-` and changes from lower to upper
case (a run of capitals is one word), each word's first letter upper-cased,
joined with spaces: `categories.profile` → `Categories Profile`,
`orderID` → `Order ID`.

### 13.4 Permanence

Route keys, permission keys and parameter names are permanent: translations
and grants are stored under them. §5.3's warning names each removal.

## 14. Search

`Table.Search` calls `g.Admit(r)` once (only public routes without a
guard), then one `tr.Search` in the request's locale over the prefix
`route.`. An entry whose text or source text contains `q.Text` (ignoring
case), whose route is in the table, passes `q.Only` and is admitted becomes
a hit, in key order, up to `q.Limit`; an empty `q.Text` matches every
entry. `Search` returns `Admit`'s error.

## 15. Tools for editors and agents

- `handlers lsp`: full document sync; diagnostics from the syntax pass on
  open and change and from both passes on save; completion of directive
  names, values and parameter names; hover; definition for `entity`. It
  negotiates UTF-8 positions when the client offers them, else UTF-16.
- `dev/vscode`: the VS Code extension, a client of `handlers lsp`, with the
  TextMate grammar for directives.
- `dev/claude-plugin`: a Claude Code plugin with the `vet -hook` hook and,
  from M6.2, the `handlers` skill; `.claude-plugin/marketplace.json` lists
  it as `handlers@epistemer-handlers`.
- `dev/gemini-extension`: a Gemini CLI extension with the same hook.

## 16. Not in v0.1

Breadcrumbs and menus; API keys; query and body parameters for admin
handlers; OpenAPI 3.1; `x-go-type` and other schemas on path parameters;
a permission shared by several routes packages, or declared outside
handlers; generated request validators; a
client SDK pipeline; a language server in the Claude Code plugin.

## 17. Amendments

- **A1** (2026-11-18, M3, Claude): `Mount.HandleHTTP` and `Mount.Writer`, plumbing for §11.5; apps don't call them.
- **A2** (2026-11-18, M3, Claude): §3.3's example calls oapi-codegen's `GetSpec`, which replaces the deprecated `GetSwagger` in v2.8.

# handlers

neozp's route package: admin routes from comment directives, API routes from
an OpenAPI 3.0 file, one `routes/v1` domain, and from it registration on
`net/http`, i18n labels and [auth](https://github.com/epistemer/auth)
permissions.

- **Admin routes** are `//h:` doc-comment directives on exported handler
  methods, checked like Go code with positions and stable diagnostics.
- **API routes** come from one OpenAPI 3.0.x document (`handlers openapi`),
  mounted over [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen)'s
  strict `std-http` server with kin-openapi request validation.
- **One route table** per surface (`Routes`, `ByKey`, typed URL builders),
  with `handlers.NewTable` and `Table.Search` for permission-filtered
  navigation search.
- **Fail closed:** the matched `handlers.Route` enters the request context
  before the guard runs, non-public routes panic at registration without a
  guard or with an undeclared permission, and prefixed surfaces register a
  catch-all `404` without `Allow`.
- **Editor and agent tools** in [`dev/`](dev): `handlers lsp`, a VS Code
  extension, a Claude Code plugin and a Gemini CLI extension.

## Modules

| Module | Package | What |
|---|---|---|
| `github.com/epistemer/handlers` | `handlers` | Runtime, `Table.Search`, and `genx.meta.yaml` generators (`routes`, `labels`, `permissions`) |
| | `handlers/handlerstest` | Fake `Guard` and `AssertPublic` for tests |
| `github.com/epistemer/handlers/authguard` | `authguard` | `Guard` over `auth` and `auth/access` |
| `github.com/epistemer/handlers/openapi` | `openapi` | Adapter for oapi-codegen's strict `std-http` server |
| `github.com/epistemer/handlers/cmd` | `cmd/handlers` | CLI (`gen`, `openapi`, `vet`, `lsp`) |

Before `v1.0.0` the API may still change; [CHANGELOG.md](CHANGELOG.md) says
how.

## Quickstart

Add `handlers` to an app on [genx](https://github.com/epistemer/genx),
[i18n](https://github.com/epistemer/i18n) and
[auth](https://github.com/epistemer/auth).

Install the CLI as a Go tool and require the runtime packages:

```sh
go get -tool github.com/epistemer/handlers/cmd/handlers
go get -tool github.com/epistemer/genx/cmd/genx
go get github.com/epistemer/handlers github.com/epistemer/handlers/authguard
```

Keep a `//go:build genx` file so `go mod tidy` retains the generator modules
before generated files exist:

<!-- quickstart:file tools.go -->
```go
//go:build genx

package main

import (
	_ "github.com/epistemer/handlers"
	_ "github.com/epistemer/i18n"
)
```

Annotate exported handler methods on one receiver type with `//h:` directives:

<!-- quickstart:file internal/admin/admin.go -->
```go
package admin

import (
	"fmt"
	"net/http"
)

type H struct{}

//h:key sign_in
//h:method GET
//h:path /sign-in
//h:desc Sign in
//h:auth public
//h:view
func (h *H) SignIn(w http.ResponseWriter, r *http.Request) error {
	_, err := fmt.Fprint(w, "sign in")
	return err
}

//h:key orders.list
//h:method GET
//h:path /orders
//h:desc Orders
//h:perm orders.manage
//h:view
//h:search
func (h *H) Orders(w http.ResponseWriter, r *http.Request) error {
	_, err := fmt.Fprint(w, "orders")
	return err
}

//h:key orders.detail
//h:method GET
//h:path /orders/{id}
//h:desc Order detail
//h:perm orders.manage
//h:view
//h:entity orders.list
//h:param id label Order ID
func (h *H) Order(w http.ResponseWriter, r *http.Request, id int64) error {
	_, err := fmt.Fprintf(w, "order %d", id)
	return err
}
```

Declare the producer and generators in `genx.yaml` and run `go tool genx`:

<!-- quickstart:file genx.yaml -->
```yaml
version: 1
producers:
  - name: admin-routes
    run:   [go, tool, handlers, gen, -prefix, /admin, -out, internal/admin/adminroutes, ./internal/admin]
    check: [go, tool, handlers, gen, -check, -prefix, /admin, -out, internal/admin/adminroutes, ./internal/admin]
    emits: [internal/admin/adminroutes/zz_adminroutes.routes.json]
use:
  - github.com/epistemer/handlers: [routes, labels, permissions]
  - github.com/epistemer/i18n: [labels]
```

```sh
go tool genx
```

That writes `zz_adminroutes.routes.json`, `zz_adminroutes_routes.go`
(`Prefix`, `Routes`, `ByKey`, `Handler`, `Register`, URL builders),
`zz_adminroutes.labels.json`, `zz_adminroutes_i18n.go` (`L10n*`) and
`zz_adminroutes_permissions.go` (`PermOrdersManage`) in
`internal/admin/adminroutes`.

### Wiring

Wire `authguard`, register the admin surface on its own `http.ServeMux` with
`auth.Cookie()`, and wrap the root mux with `i18n`'s middleware:

<!-- quickstart:file main.go -->
```go
package main

import (
	"net/http"

	"github.com/epistemer/auth"
	"github.com/epistemer/auth/access"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/authguard"
	"github.com/epistemer/i18n"

	"example.com/quickstart/internal/admin"
	"example.com/quickstart/internal/admin/adminroutes"
)

func newHandler(a *auth.Auth, ac *access.Access, tr *i18n.Translator) http.Handler {
	guard := authguard.New(a, ac)

	adminMux := http.NewServeMux()
	adminroutes.Register(adminMux, &admin.H{}, guard, handlers.WithErrors(handlers.WriteText))

	root := http.NewServeMux()
	root.Handle(adminroutes.Prefix+"/", a.Load(auth.Cookie())(adminMux))
	return tr.Middleware()(root)
}

func main() {}
```

To add a JSON API from an OpenAPI 3.0 spec, add an `openapi` producer to
`genx.yaml`:

```yaml
  - name: api-routes
    run:   [go, tool, handlers, openapi, -key-prefix, api, -out, internal/api/apiroutes, api/openapi.yaml]
    check: [go, tool, handlers, openapi, -check, -key-prefix, api, -out, internal/api/apiroutes, api/openapi.yaml]
    emits: [internal/api/apiroutes/zz_apiroutes.routes.json]
```

Then mount `oapi-codegen`'s strict `std-http` server on its own mux with
`auth.Bearer()`:

```go
apiMux := http.NewServeMux()
spec, _ := apigen.GetSpec() // oapi-codegen's embedded spec (GetSwagger before v2.8)
apiroutes.Mount(apiMux, guard, spec, func(rt *openapi.Router) {
	apigen.HandlerWithOptions(
		apigen.NewStrictHandlerWithOptions(apiImpl, nil, apigen.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  rt.RequestError,
			ResponseErrorHandlerFunc: rt.WriteError,
		}),
		apigen.StdHTTPServerOptions{
			BaseURL:          apiroutes.Prefix,
			BaseRouter:       rt,
			ErrorHandlerFunc: rt.RequestError,
		},
	)
})
root.Handle(apiroutes.Prefix+"/", a.Load(auth.Bearer())(apiMux))
```

See [examples/shop](examples/shop) for a complete application with admin pages,
an OpenAPI server (`oapi-codegen`), `Table.Search`, localized error pages,
`cfgx`, `models` and end-to-end tests.

### CI

Check generated files and run `vet` against the base branch so removed route
keys, parameter labels or permissions warn before merging:

```sh
go tool genx check
go tool handlers vet -strict -baseline origin/main ./...
```

## Editor and agent tools

| Tool | Location | What |
|---|---|---|
| Language server | `go tool handlers lsp` | Diagnostics (syntax on edit, type check on save), completion, hover, `entity` definition |
| VS Code extension | [dev/vscode](dev/vscode) | Client of `handlers lsp` and TextMate grammar for `//h:` directives |
| Claude Code plugin | [dev/claude-plugin](dev/claude-plugin) | `PostToolUse` hook (`handlers vet -hook`) and `handlers` skill (`handlers@epistemer-handlers`) |
| Gemini CLI extension | [dev/gemini-extension](dev/gemini-extension) | `AfterTool` hook (`handlers vet -hook`) and `GEMINI.md` context |

## Where to read

| | |
|---|---|
| [docs/interface.md](docs/interface.md) | The locked contract: directives, diagnostics, commands, generated Go and runtime API |
| [examples/shop](examples/shop) | Complete example application (admin, API, search, auth, i18n, models, tests) |
| [docs/api-review.md](docs/api-review.md) | Every exported identifier, generated symbol and command flag against the contract |
| [CHANGELOG.md](CHANGELOG.md) | Changes by version |
| [docs/adr/0001-routes-domain.md](docs/adr/0001-routes-domain.md) | Architecture decision records (`docs/adr/`) |
| [docs/roadmap.md](docs/roadmap.md) | Milestones and what counts as done |
| [docs/status.md](docs/status.md) | Working session log |
| [docs/reviews](docs/reviews) | Cross-model reviews |

## License

MIT

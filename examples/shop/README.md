# Toy Shop (`examples/shop`)

A complete example application showing `github.com/epistemer/handlers` working alongside every sibling library:

- **cfgx** (`internal/settings`): typed configuration (`authschema`, `i18nschema`, server listener and bbolt store path) and `//go:generate go tool cfgx gen`.
- **i18n** (`main.go`, generated `zz_*_i18n.go`): bbolt-backed runtime translations and `tr.Middleware()` request locale resolution.
- **errors** (`internal/shop`): declared domain errors (`errors.Key`) mapped to HTTP statuses and localized messages without leaking internal causes.
- **auth** & **authguard** (`main.go`, `internal/admin`, `api`): cookie sessions on `/admin`, bearer JWT tokens on `/api/v1`, each on its own `http.ServeMux` and `auth.Load` middleware, plus `auth/access` permissions guarded by `authguard.New`.
- **models** (`db/`, `sqlc.yaml`, `internal/models`): `sqlc` generates `internal/models` Go types and `codegen_request.json`; `models`' `labels` and `enums` generators run over the committed `codegen_request.json` (with no `check` command on the `sqlc` producer so CI skips building `sqlc`).
- **handlers** (`internal/admin`, `internal/adminroutes`, `api`, `apiroutes`):
  - Admin HTML pages under `/admin` using `html/template`, generated `L10n*` label variables, generated URL builders, and a custom HTML `ErrorPage` via `handlers.WithErrors`.
  - Global route search (`GET /admin/search?q=`) via `handlers.NewTable` and `Table.Search` with `authguard`.
  - JSON API under `/api/v1` generated from `api/openapi.yaml` by `oapi-codegen` (strict `std-http` server) and mounted with `apiroutes.Mount`.
- **genx** (`genx.yaml`, `tools.go`, `genx.lock`): orchestrates `cfgx`, `handlers gen`, `handlers openapi`, `models`, and `i18n` in dependency order.

## Running

From `examples/shop`:

```bash
SHOP_AUTH_ISSUER=toyshop \
SHOP_AUTH_SECRET=$(openssl rand -hex 32) \
SHOP_AUTH_COOKIE_INSECURE=true \
SHOP_DEMO_PASSWORD='orange tiger juggles 47 lamps' \
SHOP_DEMO_MAIL=$(mktemp -d) \
go run . serve
```

- Admin sign-in: `http://localhost:8080/admin/sign-in` (login `sara@toyshop.example`; emailed verification codes are written to `$SHOP_DEMO_MAIL`).
- API sign-in: `POST http://localhost:8080/api/v1/auth/sign-in` with `{"email":"sara@toyshop.example","password":"..."}`, followed by `POST /api/v1/auth/sign-in/code`.

To run the test suite and check generated files:

```bash
go test -race -count=1 ./...
go tool genx check
go tool handlers vet -strict ./...
```

## Adding a route

1. **Write the directives** above an exported method on `*admin.H` in `internal/admin/`:

   ```go
   //h:key orders.refund
   //h:method POST
   //h:path /orders/{id}/refund
   //h:desc Refund order
   //h:perm orders.manage
   //h:param id label Order ID
   func (h *H) OrderRefund(w http.ResponseWriter, r *http.Request, id ids.OrderID) error {
   	// implement after generating if you want to use adminroutes.L10nOrderRefund or URL builders
   	return nil
   }
   ```

2. **Run `go tool genx`** from `examples/shop`:

   ```bash
   go tool genx
   ```

   This runs `handlers gen` to update `internal/adminroutes/zz_adminroutes.routes.json`, then regenerates `zz_adminroutes_routes.go` (including the `adminroutes.Handler` interface and `adminroutes.OrderRefund(id)` URL builder), `zz_adminroutes.labels.json`, `zz_adminroutes_i18n.go` (`adminroutes.L10nOrderRefund`), and `zz_adminroutes_permissions.go`.

3. **Implement the method** body using the newly generated `adminroutes` label variables and URL builders, then verify everything compiles and passes `handlers vet`:

   ```bash
   go tool handlers vet -strict ./...
   go test -race ./...
   ```

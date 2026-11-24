# handlers

This Go project uses `github.com/epistemer/handlers` for route declarations via
`//h:` doc-comment directives on handler methods.

## Handler signature

Every handler in a package is an exported method on the same receiver type:

```go
func (h *H) Name(w http.ResponseWriter, r *http.Request, <path params>) error
```

Every parameter after `w` and `r` corresponds to a `{name}` or trailing
`{name...}` wildcard in `//h:path`. Supported parameter types are `string`,
`bool`, integers (except `uintptr`), types with those underlying types, or text
types (`T` implementing `encoding.TextMarshaler` and `*T` implementing
`encoding.TextUnmarshaler`) declared in another package.

## Directives

Directives are `//h:<name>` or `//h:<name> <value>` (no space after `//`):

- `//h:key <key>` — required dot-separated key (`[a-z0-9][A-Za-z0-9_]*`).
- `//h:method <METHOD>` — required: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`.
- `//h:path <path>` — required: starts with `/`, no trailing `/` (unless `/`),
  no `{$}`; supports `{name}` and trailing `{name...}`.
- `//h:desc <text>` — label text (defaults to readable key with a warning).
- `//h:auth <level>` — `public`, `session`, or `unverified` (default: route
  permission at `Verified`).
- `//h:perm <key>` — permission key (only when `auth` is default or `unverified`);
  `//h:perm <key> <text>` gives the permission its text; `//h:perm <key> from
  <package>` takes a permission another routes package declares (nothing may
  follow the package).
- `//h:body <size>` — the route's own body limit, as `20MiB` (not on `GET`).
- `//h:view [true]` — mark route as a view page.
- `//h:search [true]` — include route in search.
- `//h:entity <key>` — another route's key in the same package.
- `//h:param <name> label <text>` — label for a path parameter.

## Checking directives

Run `go tool handlers vet ./...` to check directives across packages (add
`-strict` to fail on warnings), and `go tool handlers gen ...` (or `genx`) to
regenerate route files when directives change.

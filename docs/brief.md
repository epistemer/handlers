# neozp `handlers`: design brief

> Written on 2026-11-18 in a planning session with epistemer, for the Claude Code session that starts `github.com/epistemer/handlers`. Decisions marked **epistemer** were made that day. Decisions marked **(Claude)** were settled while writing this brief and can be overturned before the interface is locked. zp appears only as counts and shapes, so this file can be committed as `docs/brief.md`.

## 0. How to use this brief

This is neozp's seventh package, after cfgx, i18n, genx, models, errors and auth. The folder `/home/epistemer/handlers` holds only this brief and the roadmap and is not yet a git repository. The module path is `github.com/epistemer/handlers`; creating the GitHub repository is epistemer's to do.

1. Read the sibling repositories for their conventions: the README, `docs/interface.md` (a locked contract with a decisions table and amendments), `docs/roadmap.md`, `docs/status.md`, `docs/adr/`, `docs/ownership.md`, `CLAUDE.md` and CI. Load the `genx`, `i18n`, `errors` and `auth` skills. The local checkouts of genx, cfgx and models may be behind `origin/main`: read their docs from there.
2. Set the module up like theirs: Go 1.26, MIT, the same CI jobs (Go 1.26 and 1.27 on Linux, macOS and Windows; gofmt, vet, staticcheck, tidy, tests with the race detector), plus a job for each tool in `dev/`.
3. Write the design records from this brief: ADR 0001 from §§1–3, `docs/interface.md` from §§4–11 for epistemer to lock after the spikes, and keep [roadmap.md](roadmap.md) current.
4. Run the spikes of M1 before the interface is locked. Each can change a decision (§13).
5. Ownership is epistemer's to grant when the work starts; the terms would be models' `docs/ownership.md`. Every working session adds an entry to `docs/status.md`.
6. zp is private and read-only. Keep its names out of this repository: the examples use a toy shop like the siblings' fixtures. Counts and shapes are fine.

## 1. What zp's routing taught

zp has two route pipelines that share nothing but the server.

**Admin pages** are declared by comment directives above handler methods: `//h:key`, `//h:method`, `//h:path`, `//h:desc`, `//h:auth`, `//h:view`, `//h:search`, `//h:entity`, and `//p[<param>]:label` for a parameter. A generator parses the package's AST and writes three files: an interface with one method per route plus a function that registers them, a typed URL builder per route, and an i18n key per route (`route.<key>`) and per labelled parameter (`route_param.<key>.<param>`). The route's details travel as protobuf bytes in the key's i18n meta, and the command palette decodes them from the translation store. About 217 routes use the pattern, and templates build every URL through the builders, never from strings. Writing routes this way works well; the people who use it want to keep it.

**The JSON API** is spec-first: a hand-written OpenAPI 3.0 file of about 2,700 lines and 34 operations, from which a third-party generator writes a typed server. Text-substitution passes over the generated code strip the version prefix and patch the JSON encoder.

The failure shapes this design has to make impossible:

| Failure shape | What this package does about it |
|---|---|
| A duplicate key and path registered twice; the router kept one in silence | Duplicate keys and duplicate method+pattern pairs are errors (§5.3) |
| Parameter directives that gofmt rewrote (`//p[x]:` became `// p[x]:`), so the parser skipped them for good | Every directive is gofmt-safe, and a near-miss is an error with a fix hint (§5.1, §5.3) |
| Directives whose values were ignored: any value of the view flag meant true, a non-boolean search flag was dropped, two auth levels and one directive had no effect | Every value is checked; a directive that does nothing doesn't exist (§5.2) |
| Descriptions pasted into generated Go without escaping, so a quote broke the build | Every literal reaches the template already encoded (§6) |
| A path parameter that didn't parse answered 500 | A bad parameter is `errors.Invalid` (400) (§7) |
| Permissions stored per route and never enforced: the check had no callers, and the route's name reached the request after the auth middleware ran | The route is in the context before the guard runs, guards come from the route table, and registration fails closed (§7, §8) |
| The command palette listed every route to every signed-in user | Search filters results through the same guard (§9) |
| Navigation and breadcrumbs maintained by hand beside the route metadata | The route table is data an app can build them from (backlog: a `parent` directive) |
| Four copies of the API spec that disagree, one of them used to generate the mobile client | One spec file; the server can serve it (§10) |
| The version prefix stripped from the code by text substitution but kept in the embedded spec | The prefix comes from the spec's `servers` entry (§10) |
| A header the server enforces that the spec doesn't describe; 403 sent where the spec says 401 | Guards come from the spec's `security`; statuses come from error kinds (§10) |
| API errors that are the code's English `Error()` text | Errors are written with `errors.Public` and `errors.KindOf` (§10) |
| An editor extension whose regular expressions accept what the generator rejects | One rule engine behind the generator, the CLI, the language server and the agent hooks (§11) |

## 2. The idea

**One domain, two producers.** Admin routes come from directives on handler methods and API routes come from an OpenAPI file, but both end as the same facts: a key, a method and pattern, parameters, who may call it, and a text people see. Both producers write those facts as a genx domain, `routes/v1`. Trivial generators turn that domain into Go, labels and permissions, so the two surfaces get the same registration, guards, labels and search.

**Directives stay, and get a compiler's discipline.** The directives are kept because they sit next to the code they describe. What changes is that they are checked like code: every fault is an error with a position, or a warning that CI can turn into one. The checks live in one engine, and every front end (the producer, `handlers vet`, the language server, the hooks for coding agents) asks that engine, so the editor, an agent and CI never disagree.

**A route is a permission.** Every guarded route declares an `auth/access` permission, unless it says it shares one. Routes fail closed: an app can't register a guarded route without a guard.

## 3. Decisions

| # | Topic | Decision | Who |
|---|---|---|---|
| D1 | Name | `github.com/epistemer/handlers`, package `handlers`. Label keys stay `route.<key>` | epistemer |
| D2 | Admin routes | Comment directives on handler methods, hardened: broken directives, duplicate keys and mismatched parameters are errors or warnings | epistemer |
| D3 | API | Spec-first: one OpenAPI file is the source | epistemer |
| D4 | Permissions | The package declares `auth/access` permissions for routes and guards them with auth | epistemer |
| D5 | Editor and agents | The VS Code extension moves into `dev/vscode` and reports faults while typing; a Claude Code plugin and a Gemini CLI extension sit beside it | epistemer |
| D6 | Pipeline | Two producers (`handlers gen`, `handlers openapi`) write `routes/v1`; trivial generators `routes`, `labels` and `permissions` read it | (Claude) |
| D7 | Domain owner | `routes/v1` lives in this repository (schema and conformance files) until a second package produces or consumes it; then it moves to genx | (Claude) |
| D8 | Router | `net/http` `ServeMux` patterns, as auth's D21. No third-party router | (Claude) |
| D9 | Handler shape | `func (h *H) X(w http.ResponseWriter, r *http.Request, <params>) error`; parameters come from the signature | (Claude) |
| D10 | Route meta | None. A generated route table replaces the protobuf meta: search joins `route.*` labels to it by key (§9). genx needs no `meta` on `labels/v1` | (Claude), explained to epistemer on 2026-11-18 |
| D11 | Auth levels | `public`, `session`, `unverified`, and the default: the route's permission at `Verified`. zp's `root` is dropped: `access.Root` already holds every permission | epistemer (2026-11-18) |
| D12 | Fail closed | The route is in the context before the guard. `Register` panics when a guarded route has no guard, and the guard refuses a permission that isn't declared | (Claude) |
| D13 | API server | Chosen by spike S2 among ogen, oapi-codegen's strict `std-http` server, and oapi-codegen's types with this package's own `routes` template | (Claude), spike |
| D14 | Errors | Handlers return errors; the package writes them through `errors.Public` and `errors.KindOf`: an app hook for HTML, the `{code, message}` envelope for JSON | (Claude) |
| D15 | Dependencies | The core depends on `errors` and `i18n`. `handlers/authguard`, the guard over auth, is a nested module, like auth's `passkey` and `oidc` | (Claude) |
| D16 | Keys | Route keys are permanent: each is an i18n key and holds permission grants. Removing one is a warning naming what it orphans | epistemer (2026-11-18) |
| D17 | Out of scope | Rendering templ, HTMX helpers, flash messages, server-sent events, nav menus and CRUD tables: later UI packages | (Claude) |
| D18 | Parameter directive | `//h:param <name> label <text>` replaces `//p[<name>]:label`, so gofmt leaves it alone | epistemer (2026-11-18) |
| D19 | Editor | The VS Code extension is a thin client of `handlers lsp`; completions and checks live in the server | epistemer (2026-11-18) |

## 4. Packages and commands

| Path | What |
|---|---|
| `handlers` | Runtime: `Route`, `From(ctx)`, `Guard`, the error writers, `Search`, declared errors. `genx.meta.yaml` with the generators `routes`, `labels`, `permissions` |
| `handlers/authguard` | Nested module: a `Guard` over `auth.Require` and `access.Can` |
| `handlers/handlerstest` | Test helpers: a fake guard, and an assertion that every non-public route is guarded |
| `internal/directive` | The rule engine: parse, check, diagnostics with positions |
| `cmd/handlers` | `gen` (directive producer), `openapi` (spec producer), `vet` (check, write nothing), `lsp` (language server) |
| `dev/vscode`, `dev/claude-plugin`, `dev/gemini-extension` | §11 |

## 5. Directives

### 5.1 Grammar (Claude: final at the freeze)

Go treats a comment of the form `//word:value` as a directive: gofmt leaves it alone and godoc hides it. Every directive takes that form:

```go
//h:key categories.profile
//h:method GET
//h:path /categories/{id}
//h:desc Category detail
//h:param id label Category ID
//h:entity categories.search
//h:perm categories.manage
func (h *H) Category(w http.ResponseWriter, r *http.Request, id int64) error {
```

- zp's vocabulary stays: `key method path desc auth view search entity`. `view` and `search` are flags without a value.
- `//p[<param>]:label` becomes `//h:param <param> label <text>`, because the bracket form isn't a directive to gofmt.
- `perm` is new (§8). `bundle` goes: it never did anything.
- A package's mount prefix (zp's `/admin`) is a producer flag, not a directive.
- Paths use `ServeMux` wildcards: `{id}`, `{path...}`.

### 5.2 Values

| Directive | Value | Default |
|---|---|---|
| `key` | `[a-z0-9_]+(\.[a-z0-9_]+)*` | required |
| `method` | `GET POST PUT PATCH DELETE` (any case) | required |
| `path` | a `ServeMux` path; its wildcards must match the signature's parameters | required |
| `desc` | free text, one line | the key made readable, with a warning |
| `auth` | `public session unverified` | the route's permission at `Verified` |
| `perm` | a permission key | the route key |
| `view`, `search` | none | off |
| `entity` | another route's key | none |
| `param` | `<name> label <text>` | the parameter's name made readable |

### 5.3 Diagnostics

Every diagnostic has a position (`file:line:col`, as `go vet` prints) and a stable message, tested word for word.

**Errors** (the producer writes nothing):
- an unknown directive, or a near-miss (`// h:`, `//H:`, `//h key`, `//p[...]`), with the fix;
- a directive given twice on one handler, or a bad value;
- a missing `key`, `method` or `path`;
- a duplicate key, or a duplicate method and pattern;
- two patterns `ServeMux` would refuse as conflicting;
- a wildcard without a parameter, or a parameter without a wildcard;
- a parameter type the binder can't parse (supported: strings, integers, booleans, named types over them, and `encoding.TextUnmarshaler`);
- `entity` or `param` naming something that doesn't exist;
- a directive on a function that isn't a handler (wrong signature, unexported, not a method).

**Warnings** (on stderr; `-strict` turns them into errors, for CI):
- a missing `desc`;
- a key in the committed `routes/v1` file that is no longer declared: its translations and permission grants are orphaned;
- `perm` naming a permission that only this route uses.

## 6. `routes/v1`

A sketch; the schema is M1.4's.

```json
{
  "domain": "routes/v1",
  "producer": "handlers",
  "package": {"name": "adminroutes", "dir": "internal/admin/adminroutes"},
  "handler": {"import": "example.com/shop/internal/admin", "type": "H"},
  "surface": "html",
  "prefix": "/admin",
  "routes": [{
    "key": "categories.profile", "method": "GET", "pattern": "/categories/{id}",
    "name": "Category", "text": "Category detail",
    "auth": "permission", "permission": "categories.manage",
    "view": false, "search": false, "entity": "categories.search",
    "params": [{"name": "id", "go": "id", "type": "int64", "parse": "int", "label": "Category ID"}]
  }]
}
```

- Producers resolve every Go name (`name`, `go`, the types) and give string literals as JSON, which is valid Go: templates only render (genx ADR 0001 §7).
- The generated code goes into its own package, as zp's did, so templates can import the URL builders without importing the handlers.
- Within a major version, fields are only added.

## 7. Generated Go (`routes`)

```go
package adminroutes

type Handler interface {
	Category(w http.ResponseWriter, r *http.Request, id int64) error
	// one method per route
}

// Register adds every route to mux. Each route's chain is: the route into
// the context, the guard for its auth, parameter binding, the handler, the
// error writer. It panics if a route needs a guard and g is nil.
func Register(mux *http.ServeMux, h Handler, g handlers.Guard, opts ...handlers.Option)

func Category(id int64) string // "/admin/categories/42", parameters escaped

var Routes = []handlers.Route{ /* … */ }
func ByKey(key string) (handlers.Route, bool)
```

- A parameter that doesn't parse is `handlers.ErrBadParam` (kind `Invalid`, 400).
- A handler's error goes to the error writer (D14).

## 8. Permissions (`permissions`)

```go
package adminroutes

var (
	PermCategoriesManage = access.Permission("categories.manage", "Manage categories")
	PermOrdersRefund     = access.Permission("orders.refund", "Refund orders", auth.Verified)
)
```

- One permission per guarded route, keyed by the route key, with the route's text and the level its `auth` names.
- `//h:perm` lets routes share one permission, such as a form page and the action it posts to. A shared permission's text is the first route's `desc` unless the routes agree on another. Census S1 counts the page/action pairs and decides whether pairing is the default.
- `access.Permission` registers the i18n key `permission.<key>` itself, so these texts aren't in the labels file.
- `authguard.New(a *auth.Auth, ac *access.Access) handlers.Guard`:
  - `public`: no guard;
  - `session`: `a.Require()`;
  - `unverified`: `a.Require(auth.Unverified)`;
  - a permission: `a.Require(ac.Can(p))`, which also enforces the permission's level (auth A18).

  It finds `p` by key among `access.Permissions()`. A key that isn't declared stops `Register`.
- `handlerstest` asserts that every non-public route of a table is guarded and that every permission it names is declared.

## 9. Labels and search

- `labels` emits `labels/v1`:
  - `route.<key>`, with the route's text and variable `L10n<Name>`;
  - `route_param.<key>.<param>`, with the parameter's label and variable `L10n<Name><Param>`.

  i18n's `labels` generator writes the Go. There is no `meta`.
- `handlers.Search(tr, code, table, q)` runs one `tr.Search` over the prefix `route.` with `Contains` set to the query. Its `Match` looks each key up in the generated table and keeps the routes the query asks for (searchable, view, entity) that the guard admits for the request.
- **Cost.** `Search` walks the prefix in one bbolt read transaction and unmarshals each catalog entry; that walk is the same with or without meta and dominates. zp's way added a protobuf decode per entry. The table lookup is a Go map access of tens of nanoseconds, so the join costs less than the decode. M2.6 benchmarks it on 300 routes.

## 10. The API

- One committed `openapi.yaml`. `servers[0].url` gives the mount prefix (`/api/v1`), and the server is mounted with it: no text substitution.
- `handlers openapi` reads the spec and writes `routes/v1` with `surface: json`. It checks that:
  - every operation has a unique `operationId`;
  - every security requirement names a declared scheme;
  - every `x-permission` is a valid key;
  - every operation's error responses use the spec's `Error` schema, `{code, message}`.
- Guards come from the spec:
  - `security: []` is `public`;
  - a bearer scheme is `session`;
  - `x-permission: <key>` names the permission, whose default is the route key.

  The route key is `x-route-key` or else the `operationId` made dotted (`placeOrder` → `place_order`), with a prefix the producer is given (`api.`).
- The typed server comes from the generator spike S2 picks; that spike also decides whether `routes` renders the API's registration too. Whatever wins, the server must:
  - run on `net/http`;
  - send errors through D14's JSON writer;
  - run the guard from the route table;
  - validate requests against the spec;
  - map money to a decimal type;
  - accept multipart bodies.
- Optionally, the server serves the spec, so clients generate from the one file.
- API keys aren't covered: auth leaves them out (backlog).

## 11. Tools for editors and agents (`dev/`)

| Path | What |
|---|---|
| `cmd/handlers lsp` | A language server over the rule engine: diagnostics as you type, completion of directives, values and parameter names, hover with the directive's doc, go to definition for `entity` keys |
| `dev/vscode` | zp's extension (TextMate grammar and completions), imported with `git subtree` to keep its history and renamed. It becomes a thin client of `handlers lsp`; its completions move into the server, so the two can't drift. CI builds the `.vsix` as an artifact, never committed |
| `dev/claude-plugin` | A Claude Code plugin: `.lsp.json` runs `handlers lsp` on `.go` files, so Claude sees diagnostics after each edit; `hooks/hooks.json` has a PostToolUse hook on `Edit\|Write` that runs `handlers vet` on the edited file and answers `{"decision": "block", "reason": …}` when it fails, which shows the faults beside the edit without undoing it; `skills/handlers/` is the package's skill. `.claude-plugin/marketplace.json` at the repository root, with a `git-subdir` source, makes it installable with `claude plugin install handlers@epistemer-handlers` |
| `dev/gemini-extension` | A Gemini CLI extension with the same `vet` hook and a context file, for work delegated to agy |

Whether a second language server can attach to `.go` beside gopls in Claude Code is unverified; S3 tests it, and if it can't, the hook carries the checks alone.

## 12. Tests

- Each generator is golden-tested with `genxtest.Render` and `RenderParams`. A second run gives the same bytes, and every labels file passes genx's conformance files.
- Every diagnostic has a test with its exact message. zp's real faults are rebuilt in a toy package: a duplicate key, a gofmt'd `// p[...]`, a parameter whose name differs from its wildcard, two conflicting patterns.
- A nested test module compiles and vets the generated code, and checks with `httptest`:
  - a bad parameter gives 400;
  - an unknown route gives 404 as HTML and as JSON;
  - with `authtest`, a missing permission gives 403 and a granted one 200;
  - a public route needs no session;
  - `Register` without a guard panics.
- `i18ntest` shows a translation set on a generated `route.*` key, and `Search` finds it in that locale, filtered by the guard.
- The language server has protocol tests (open, change, diagnostics). The VS Code extension and the plugin each have a CI job, including `claude plugin validate`.

## 13. Open questions, each with a default

| # | Question | Default | Settled by |
|---|---|---|---|
| Q1 | Do a form page and the action it posts to share a permission by default? | No; `//h:perm` pairs them | S1's count of pairs |
| Q2 | Name of the "signed in, no permission" level | `session` | the freeze |
| Q3 | Keep the `h:` prefix? | Yes: habit, and the extension's grammar | the freeze |
| Q4 | Where the generated Go goes | A sibling package, `dir` and `package` parameters as in models | the freeze |
| Q5 | Does the API use the third-party server, or this package's `routes` template over third-party types? | Whichever S2 shows needs less glue to meet §10 | S2 |
| Q6 | A second language server on `.go` in Claude Code | Ship it if S3 shows it attaches; otherwise only the hook | S3 |

## 14. Out of v0.1

Breadcrumbs from a `parent` directive; nav and menu metadata; API keys; a client SDK pipeline from the one spec; a shared `permissions/v1` domain; a golangci-lint plugin; anything that renders.

## 15. Risks to check early

| Risk | Likelihood / impact | Check |
|---|---|---|
| A zp route shape the new grammar can't express, or a pattern `ServeMux` refuses | Medium / medium | S1 runs every route through the draft rules |
| No OpenAPI generator meets §10 without patches | Low / high | S2's kill criterion: if none does, `routes` renders the API too, over oapi-codegen's types |
| A language server on every keystroke is slow on a large package | Low / medium | S3 measures it on 200+ routes |
| Permission per route gives too many to manage | Medium / low | S1's count; `//h:perm`; groups in `auth/access` |
| Key renames orphan grants and translations | Medium / high | D16's warning, and the committed `routes/v1` diff in review |

## 16. Roadmap

[roadmap.md](roadmap.md).

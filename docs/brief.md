# neozp `handlers`: design brief

> **History.** [ADR 0001](adr/0001-routes-domain.md), ADRs 0002–0004 and [interface.md](interface.md) replace this brief. The spikes and reviews changed several things after it was written, so where they differ, they are right. Kept as the record of what was known before the work began.

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
| D6 | Pipeline | Two producers (`handlers gen`, `handlers openapi`) write `routes/v1`; trivial generators `routes`, `labels` and `permissions` read it | (Claude), accepted with the plan on 2026-11-18 (ADR 0001) |
| D7 | Domain owner | `routes/v1` lives in this repository (schema and conformance files) until a second package produces or consumes it; then it moves to genx | (Claude), accepted with the plan on 2026-11-18 (ADR 0001) |
| D8 | Router | `net/http` `ServeMux` patterns, as auth's D21. No third-party router | (Claude) |
| D9 | Handler shape | `func (h *H) X(w http.ResponseWriter, r *http.Request, <params>) error`; parameters come from the signature | (Claude) |
| D10 | Route meta | None. A generated route table replaces the protobuf meta: search joins `route.*` labels to it by key (§9). genx needs no `meta` on `labels/v1` | (Claude), explained to epistemer and accepted with the plan on 2026-11-18 (ADR 0001) |
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
| `key` | dot-separated segments, each `[a-z0-9][A-Za-z0-9_]*`: snake case or camelCase (ADR 0002) | required |
| `method` | `GET POST PUT PATCH DELETE` (any case) | required |
| `path` | a `ServeMux` path that starts with `/` and, unless it is the root, doesn't end with one; its wildcards must match the signature's parameters by name. The pattern is the prefix followed by the path | required |
| `desc` | free text, one line, not empty | the key made readable, with a warning |
| `auth` | `public`, `session` (signed in, no permission), `unverified` (the route's permission, declared at `auth.Unverified`) | the route's permission at `auth.Verified` |
| `perm` | a permission key | the route key |
| `view`, `search` | none, or `true`; any other value, `false` included, is an error (ADR 0002) | off |
| `entity` | another route's key | none |
| `param` | `<name> label <text>` | the parameter's name made readable |

A route's parameters keep the handler signature's order.

### 5.3 Diagnostics

Every diagnostic has a position (`file:line:col`, as `go vet` prints) and a stable message, tested word for word. The engine scans every comment of a file, not only the doc comments of functions, so a block of directives a blank line away from its function is an error, not lost.

**Errors** (the producer writes nothing):
- an unknown directive, or a near-miss, with the fix. A near-miss is a comment line that, after `//`, has optional spaces, `h` or `H`, optional spaces and a `:` (`// h:key`, `//H:key`, `//h :key`); or `//`, optional spaces, `h`, one space and a directive name (`//h key …`); or `//`, optional spaces and `p[` (zp's old parameter form). Ordinary comments, such as `// Handler serves…`, never match;
- a directive given twice on one handler, or a bad value (a flag's value other than `true`, an empty `desc`, a path without a leading `/` or with a trailing one);
- two keys of one package whose Go names come out the same (`pricingPlans`, `pricing_plans`);
- a block of directives that isn't the doc comment of a function;
- a missing `key`, `method` or `path`;
- a duplicate key, or a duplicate method and pattern;
- a pattern `ServeMux` refuses, or two it would refuse as conflicting. The engine registers every pattern into a scratch `ServeMux` and turns its panic into the diagnostic, so it agrees with Go's rules (partial-segment wildcards, a wildcard named twice, conflicts) by construction. `handlers vet` given several packages checks their patterns together;
- a wildcard without a parameter, or a parameter without a wildcard;
- a parameter type the binder can't parse. Supported: strings, integers, booleans, named types over them, and types whose pointer implements `encoding.TextUnmarshaler` and whose value implements `encoding.TextMarshaler`, so URL builders can format them;
- a parameter type declared in the handler's own package, which would make the generated package import it (§6);
- `entity` or `param` naming something that doesn't exist;
- `perm` on a route whose `auth` is `public` or `session`;
- a directive on a function that isn't a handler (wrong signature, unexported, not a method).

**Warnings** (on stderr; `-strict` turns them into errors, for CI):
- a missing `desc`;
- a route key, permission key or parameter label that the baseline has and the source no longer declares: its translations and permission grants are orphaned. The baseline is the committed `routes/v1` file, or with `-baseline <git ref>` that file at the ref, which is how CI sees a removal the developer already regenerated;
- `perm` naming a permission that only this route uses (often a typo).

## 6. `routes/v1`

A sketch; the schema is M1.4's.

```json
{
  "domain": "routes/v1",
  "producer": "handlers",
  "package": {"name": "adminroutes", "dir": "internal/admin/adminroutes"},
  "surface": "html",
  "prefix": "/admin",
  "routes": [{
    "key": "categories.profile", "method": "GET", "pattern": "/categories/{id}",
    "name": "Category", "var": "L10nCategory", "text": "Category detail",
    "auth": "permission", "permission": "categories.manage",
    "view": false, "search": false, "entity": "categories.search",
    "params": [{"name": "id", "go": "id", "var": "L10nCategoryID",
                "type": "int64", "import": "", "parse": "int", "kind": "segment",
                "label": "Category ID"}]
  }],
  "permissions": [{"key": "categories.manage", "var": "PermCategoriesManage",
                   "text": "Manage categories", "level": "verified"}]
}
```

- Producers resolve every Go name (`name`, `var`, `go`, the types and their `import` paths) and every derived value: the deduplicated `permissions` with their text and level, a parameter's `kind` (`segment` or `rest` for `{path...}`). Templates only render (genx ADR 0001 §7).
- Templates write every string literal with the built-in `printf "%q"`, which always gives valid Go. JSON escaping isn't relied on.
- The generated package never imports the handler package: `Register` takes the `Handler` interface, and parameter types from the handler's package are refused (§5.3). An app that wants the compile-time check writes `var _ adminroutes.Handler = (*H)(nil)` in the handler package.
- The schema is `schema/routes.v1.json`; `internal/routesv1.Check` enforces it and the rules across fields in `internal/routesv1/rules.md`, against the conformance files in `testdata/routes` (M1.4).
- Within a major version, fields are only added. A generator, `Register` and every `Guard` fail closed on an `auth` or `surface` value they don't know: they error or panic, never fall through to a default.

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
- The pattern `/` under a prefix is registered as `{$}`, so it doesn't catch the whole subtree.
- `Register` adds a catch-all under the prefix, so an unknown path (404) or a known path with another method (405, with `Allow`) is answered by the error writer in the surface's format, not by `ServeMux`'s plain text.
- URL builders escape a `segment` parameter with `url.PathEscape` and a `rest` parameter segment by segment, keeping its slashes.
- The full runtime API (`Guard`, `Option`, the context helper, binding helpers, error-writer options) is written at the freeze.

## 8. Permissions (`permissions`)

```go
package adminroutes

var (
	PermCategoriesManage = access.Permission("categories.manage", "Manage categories")
	PermOrdersRefund     = access.Permission("orders.refund", "Refund orders", auth.Unverified)
)
```

- One permission per guarded route, keyed by the route key, with the route's text. `unverified` declares it at `auth.Unverified`; the default is `auth.Verified`.
- `//h:perm` lets routes share one permission, such as a form page and the action it posts to. The producer resolves its text: the first route's `desc` unless the routes agree on another. Census S1 counts the page/action pairs and decides whether pairing is the default.
- The generator reads every `routes/v1` file of the app together and stops with an error when two packages declare one permission key, instead of leaving it to `access.Permission`'s panic at startup.
- `access.Permission` registers the i18n key `permission.<key>` itself, so permission texts are the one generated text that isn't in a labels file. auth owns that key, and declaring through it is how auth's permissions are declared.
- `authguard.New(a *auth.Auth, ac *access.Access) handlers.Guard`:
  - `public`: no check;
  - `session`: `a.Check(ctx)`, a verified session;
  - a permission: `a.Check(ctx, ac.Can(p))`, which also enforces the permission's level (auth A18).

  It uses `Check` rather than `Require`'s middleware, so a refusal (401 or 403) goes through handlers' error writer and gets the surface's format: an HTML page or the JSON envelope. It finds `p` by key among `access.Permissions()`. A key that isn't declared stops `Register`.
- The app wraps the mux with `a.Load(...)`, as auth requires, so `Check` finds the session in the context.
- `Guard` also admits routes for a request in bulk: `Admit(r *http.Request) (func(handlers.Route) bool, error)` reads the session once and checks each distinct permission at most once, memoized (`ac.Check`, one bbolt read each). `Search` uses it. If M2.6's benchmark shows those reads matter, a bulk read of a user's permissions is an amendment to ask of auth.
- `handlerstest` gives a fake guard, and asserts that every non-public route of a table is guarded and that every permission it names is declared.

## 9. Labels and search

- `labels` emits `labels/v1`:
  - `route.<key>`, with the route's text and variable `L10n<Name>`;
  - `route_param.<key>.<param>`, with the parameter's label and its variable.

  The producer resolves both variable names. i18n's `labels` generator writes the Go. There is no `meta`.
- `handlers.Search(r, g, tr, code, table, q)` calls `g.Admit(r)` once, then runs one `tr.Search` over the prefix `route.` with `Contains` set to the query. Its `Match` looks each key up in the generated table and keeps the routes the query asks for (searchable, view, entity) that the admit check passes.
- **Cost.** `tr.Search` walks the prefix in one bbolt read transaction and unmarshals each catalog entry; that walk is the same with or without meta and dominates. zp's way added a protobuf decode per entry. Here each entry costs a map lookup in the table and a memoized permission check, and the session is read once per query. M2.6 benchmarks the whole call, admit check included, on 300 routes: under 1 ms on the development machine.

## 10. The API

- One committed `openapi.yaml`. The mount prefix is the path of `servers[0].url`, parsed as a URL (`https://host/api/v1` and `/api/v1` both give `/api/v1`; no `servers` gives none). No text substitution.
- The OpenAPI version is 3.0 (ADR 0003): oapi-codegen and kin-openapi support it.
- `handlers openapi` reads the spec and writes `routes/v1` with `surface: json`. It checks that:
  - every operation has a unique `operationId`;
  - every security requirement names a declared scheme;
  - every `x-permission` is a valid key;
  - every operation's error responses use the spec's `Error` schema, `{code, message}`.
  - every path parameter's name is a Go identifier, no segment holds two parameters, and no parameter sets `allowReserved`, so `ServeMux` can route it (ADR 0003).
- Guards come from the spec, failing closed:
  - an operation's own `security`, else the spec's top-level `security`; an operation with neither is an error;
  - `security: []` is `public`, and `x-permission` beside it is an error;
  - a bearer scheme is the route's permission: `x-permission: <key>`, defaulting to the route key. `x-auth: session` opts out to a signed-in session with no permission.
- The route key is `x-route-key`, else derived from the `operationId`: split at case changes (a run of capitals is one word: `getUserID` → `get_user_id`), at `-` and `_`, lowercased and joined with `_`, then prefixed with what the producer is given (`api.`). Two operations deriving one key is an error. An `operationId` is already the API's public name, so tying the key to it adds no new churn; `x-route-key` pins a key across a rename.
- The typed server is oapi-codegen's strict `std-http` server (ADR 0003). handlers passes it an adapter as its `BaseRouter`, which registers each operation on the app's `ServeMux` behind the route, the guard and kin-openapi's validation of the request, in that order, before oapi-codegen binds parameters. The criteria it met:
  - run on `net/http`, with **one path matcher**: each operation registered on the app's `ServeMux` with the route-into-context, guard and error-writer chain, or found from `r.Pattern` before the guard. A second router behind the mux would let the two disagree on paths;
  - send errors through D14's JSON writer;
  - validate requests against the spec;
  - map money to a decimal type;
  - accept multipart bodies.
- Optionally, the server serves the spec, so clients generate from the one file.
- API keys aren't covered: auth leaves them out (backlog).

## 11. Tools for editors and agents (`dev/`)

| Path | What |
|---|---|
| `cmd/handlers lsp` | A language server over the rule engine: diagnostics as you type, completion of directives, values and parameter names, hover with the directive's doc, go to definition for `entity` keys |
| `dev/vscode` | zp's extension (TextMate grammar and completions), copied without its history and renamed. It becomes a thin client of `handlers lsp`; its completions move into the server, so the two can't drift. CI builds the `.vsix` as an artifact, never committed |
| `dev/claude-plugin` | A Claude Code plugin: `hooks/hooks.json` has a PostToolUse hook on `Edit\|Write` that runs `handlers vet` on the edited file's package and answers `{"decision": "block", "reason": …}` when it fails, which shows the faults beside the edit without undoing it; from M6.2, `skills/handlers/` is the package's skill. It ships no language server: Claude Code gives `.go` to one server, and that should stay gopls (ADR 0004). `.claude-plugin/marketplace.json` at the repository root, with a `git-subdir` source, makes it installable with `claude plugin install handlers@epistemer-handlers` |
| `dev/gemini-extension` | A Gemini CLI extension with the same `vet` hook and a context file, for work delegated to agy |

- The engine has two passes. The syntax pass reads only the AST: grammar, values, duplicates, wildcards against parameter names, patterns, `entity`. It runs on every change, also on a buffer that doesn't compile. The type pass type-checks the package for parameter types; the language server runs it on save, debounced and cached.
- The syntax pass takes about 3 ms on 200 routes and the type pass about 300 ms (ADR 0004). `handlers lsp` has no LSP library: it implements the few messages it uses.

## 12. Tests

- Each generator is golden-tested with `genxtest.Render` and `RenderParams`. A second run gives the same bytes, and every labels file passes genx's conformance files.
- Every diagnostic has a test with its exact message. zp's real faults are rebuilt in a toy package: a duplicate key, a gofmt'd `// p[...]`, a parameter whose name differs from its wildcard, two conflicting patterns.
- Every module (the root, `authguard`, the nested test module, the example) runs its tests with the race detector in CI.
- A nested test module compiles and vets the generated code, and checks with `httptest`:
  - a bad parameter gives 400;
  - an unknown route gives 404 and a wrong method 405, as HTML and as JSON;
  - a refusal from the guard gives 401 or 403 in the surface's format;
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
| Q5 | Does the API use the third-party server, or this package's `routes` template over third-party types? | Answered by ADR 0003: oapi-codegen's strict server, through handlers' adapter | S2 |
| Q6 | A second language server on `.go` in Claude Code | Answered by ADR 0004: only the hook; Claude Code gives `.go` to one server | S3 |

## 14. Out of v0.1

Breadcrumbs from a `parent` directive; nav and menu metadata; API keys; a client SDK pipeline from the one spec; a shared `permissions/v1` domain; a golangci-lint plugin; anything that renders.

## 15. Risks to check early

| Risk | Likelihood / impact | Check |
|---|---|---|
| A zp route shape the new grammar can't express, or a pattern `ServeMux` refuses | Medium / medium | S1 runs every route through the draft rules |
| No OpenAPI generator meets §10 without patches | Low / high | S2's kill criterion: if none does, `routes` renders the API too, over oapi-codegen's types |
| A language server on every keystroke is slow on a large package | Medium / medium | The syntax pass alone runs on change (§11); S3 measures both passes on 200+ routes |
| Permission per route gives too many to manage | Medium / low | S1's count; `//h:perm`; groups in `auth/access` |
| Key renames orphan grants and translations | Medium / high | D16's warning against a git baseline in CI, and the committed `routes/v1` diff in review |

## 16. Roadmap

[roadmap.md](roadmap.md).

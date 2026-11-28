# ADR 0008: a parameter's kind and source, and a link from values

| | |
|---|---|
| Status | Drafted 2026-11-28, waiting on epistemer's lock (API design). Amendment A9, for v0.5.0. Reviewed: [0009](../reviews/0009-param-sources.md) |
| Date | 2026-11-28 |
| Roadmap item | neozp decision H5 (2026-11-28) and [neozp ADR 0002](https://github.com/epistemer/neozp/blob/main/docs/adr/0002-route-search-parameters.md); Confab's F13 |
| Deciders | epistemer decides. The answers of 2026-11-28 settle the what; questions P1–P3, P9, P11 and A9.1–A9.4 below are the how. Claude proposes |

## Context

zp's global search reached a route with parameters only through an
*entity*: `//h:entity <search route>` on a detail page. The search route's
own handler turned the chosen id into the detail page's URL. handlers kept
`entity` as a route-level key, which nothing reads at run time. Confab used
it to name a list view (`//h:entity events`). `Table.Search` returns
`Hit{Route, Text}`, with no link, so Confab marks only routes without
parameters (F13).

epistemer decided on 2026-11-28 (neozp `docs/decisions.md`, ADR 0002):

1. `//h:search` stays opt-in. Any GET page may be marked, with or without
   parameters, and a route with parameters opens a form for them.
2. A parameter names its source, a suggestions route:
   `//h:param <name> from <key>` (zp's `p[id]:entity`). The route-level
   `//h:entity` is retired.
3. A nested ui module, `ui/palette`, builds the form and the palette from
   handlers' facts. ui chooses the widgets.

The form needs three facts that only handlers has:

- **A parameter's kind.** The type pass already resolves each one to
  `parse` in `routes/v1`, but the runtime `Param` drops it.
- **Its source.** This is new, the directive of answer 2.
- **A way to turn the filled strings into a `Link`,** parsed exactly as the
  binder parses them. A text type's `UnmarshalText` can only be called from
  the generated package, which imports the type.

## Decision

### 1. The `param` directive takes a source

```
//h:param <name> [from <key>] [label <text>]
```

At least one of `from` and `label` is given. `from` comes first, as in
`perm`, so a label can hold any text. It is still once per parameter.

```go
//h:key events.edit
//h:method GET
//h:path /events/{id}/edit
//h:param id from events.search label Event
//h:search
func (h *H) EventsEdit(w http.ResponseWriter, r *http.Request, id ids.EventID) error {
```

The source is a route of the same package that:

- has the method `GET`;
- has no parameters (P3);
- isn't the route itself.

It answers ui's suggestions protocol (`components.Suggestions`), but
handlers checks only the facts above.

`//h:entity` is retired. A line that uses it is an error that names the
replacement (§5 below).

### 2. `search` and `view` on GET only

`search` and `view` describe pages, so either one on a route that isn't
`GET` is an error. No route in Confab or in `examples/shop` does this today.

### 3. `routes/v1`

- A parameter gains an optional `from`, a route key.
- A route's `entity` is no longer written. Readers ignore it in older
  files.

`entity` was optional and the schema doesn't close
`additionalProperties`, so the domain stays `v1` and older files stay
valid. `internal/routesv1/rules.md` gains these rules:

- `from` names a GET route of the same file, without parameters;
- `from` is never the parameter's own route.

The OpenAPI producer writes no `from` (A9.4).

### 4. The runtime `Param` and `Route`

```go
// ParamKind is how a parameter is parsed: routes/v1's parse.
type ParamKind uint8

const (
	KindString ParamKind = iota // string and named string types; a Rest parameter is always KindString (gen already refuses another type)
	KindBool
	KindInt  // int, int8 … int64 and named types over them; Bits says which
	KindUint // uint, uint8 … uint64 and named types over them; Bits says which
	KindText // a type with UnmarshalText and MarshalText
)

func (k ParamKind) String() string // "string", "bool", "int", "uint", "text"

type Param struct {
	Name  string    // the wildcard's name
	Rest  bool      // {name...}
	Label i18n.TKer // route_param.<key>.<name>
	Kind  ParamKind // (A9)
	Bits  int       // (A9) 8, 16, 32 or 64 for KindInt and KindUint (int and uint: strconv.IntSize, written as such); 0 otherwise
	From  string    // (A9) the key of the route that suggests values, or ""
	// Canon (A9) reads a KindText value as the binder does and returns it
	// as MarshalText writes it, unescaped; nil unless Kind is KindText.
	// Generated.
	Canon func(string) (string, error)
}
```

`Route.Entity` is removed. An entity route is derived, not declared: a
searchable route whose single parameter has a `From` (neozp ADR 0002).

The generated table fills the new fields:

```go
Params: []handlers.Param{
	{Name: "id", Rest: false, Label: L10nEventsEditId, Kind: handlers.KindInt, Bits: 64, From: "events.search"},
	{Name: "locale", Label: L10nTranslationsLocale, Kind: handlers.KindText, From: "locales.search",
		Canon: func(s string) (string, error) {
			var v ids.Locale
			if err := v.UnmarshalText([]byte(s)); err != nil {
				return "", err
			}
			return handlers.Text(v), nil // MarshalText's bytes, not escaped
		}},
},
```

`Canon` is the one piece of the binder that generic code can't perform
(A9.1).

### 5. A link from values

```go
// Link returns a link to the route with this key, its parameters taken
// from v by name, each parsed as the binder parses it and written as the
// route's builder writes it. A query left in v is dropped. An unknown key
// is ErrNotFound; a missing or bad value is ErrBadParam with the
// parameter's label.
func (t *Table) Link(key string, v url.Values) (Link, error)
```

Each parameter is handled by its kind:

| Kind | Handling |
|---|---|
| KindString | Written as given |
| KindBool | `strconv.ParseBool`, then `FormatBool` |
| KindInt | `strconv.ParseInt(s, 10, Bits)`, then `FormatInt` |
| KindUint | `strconv.ParseUint(s, 10, Bits)`, then `FormatUint` |
| KindText | `Canon` |

Some values are `ErrBadParam` whatever the kind:

- a missing value, or one given twice;
- an empty value, which would leave an empty segment and so miss the
  route;
- a Rest value that starts or ends with `/` or has an empty segment, which
  would also miss the route (and could start the URL with `//`).

`middleware.Redirect` refuses a URL that isn't local (`ErrNotLocal`) in any
case.

A value is escaped with `Segment`, or with `Rest` for a rest parameter. For
the same values, the link's `URL()` equals the builder's and its `Route()`
equals the builder's route. The link points at the table's copy of the
route, so `==` with a builder's link is false (A9.2). `Link` doesn't consult
a guard, and it builds a link to any route of the table. `ui/palette`
checks that the target is a searchable GET page and admits it, before it
shows a form and before it redirects. A caller finds a route's parameters
with the existing `ByKey`.

### 6. A hit carries a link

```go
type Hit struct {
	Route Route
	Text  string // the label in the request's locale
	Link  Link   // (A9) the route's link when it has no parameters; else the zero Link
}
```

This is H5's default. A hit for a route with parameters has no link; the
palette opens the form or the entity flow for it.

### 7. Diagnostics

| Case | Message |
|---|---|
| `param` without `from` or `label` | `h:param: write "h:param <name> [from <key>] [label <text>]"` (replaces `h:param: write "h:param <name> label <text>"`) |
| `from`'s value isn't a key | `h:param <name>: "<v>" is not a key` |
| `from` unknown | `h:param <name>: no route with key "<k>"` |
| `from` naming its own route | `h:param <name>: from names the route itself` |
| A source that isn't GET | `h:param <name>: <k> is <METHOD>, and a source must be GET` |
| A source with parameters | `h:param <name>: <k> has parameters, and a source can't (yet)` |
| `entity` | `h:entity is retired: write "h:param <name> from <key>" on the parameter` (replaces every `entity` diagnostic) |
| `search` or `view` not on GET | `h:<name> on a <METHOD> route: only GET pages are searched` |

`handlers lsp`:

- completes `from` after the parameter's name;
- completes route keys after `from`;
- offers go-to-definition on `from`'s key, as it does today for `entity`;
- hover says what a source is.

## Questions for epistemer

Each question has a default. ⚠ marks a reversal of something locked.

| # | Question | Default |
|---|---|---|
| P1 | Kind names: `handlers.ParamKind` with `KindString, KindBool, KindInt, KindUint, KindText`, and `Rest` stays a bool. The bare names would collide with `handlers.Text`, the function, and claim generic words in the package | Yes |
| P2 | `(*Table).Link(key string, v url.Values) (Link, error)`: `ErrNotFound` for an unknown key, `ErrBadParam` with the parameter's label | Yes |
| P3 | A source has no parameters this round, and it is in the same routes package. Dependent sources, such as a talk filtered by its event, and a source in another package, come later | Yes |
| P9 | Query parameters stay out (§16) | Yes |
| P11 | An entity route must be marked `//h:search`, like any other | Yes |
| A9.1 | `Param.Canon`, a generated `func(string) (string, error)` on Text parameters. Alternative: a generated resolver per route, `Route.Fill`, which handles every kind in generated code | `Canon`: the rest is generic and tested once |
| A9.2 | `Table.Link`'s link points at the table's copy of the route, so it isn't `==` to a builder's link; `URL()` and `Route()` agree | Yes |
| A9.3 | ⚠ Reverses §4.2's `entity` and its diagnostics, and removes `Route.Entity`. The `search`/`view`-on-GET check is new, and it refuses nothing in Confab | Yes |
| A9.4 | The JSON surface gets `Kind`, `Bits` and `Canon`, but no `from`, which has no extension (no `x-from`), since the palette searches admin pages only | Yes |

## Consequences

- A breaking minor release, `v0.5.0`, of all four modules:
  - the root: `ParamKind`, `Param`'s fields, `Table.Link`, `Hit.Link`, and
    `Route.Entity` gone;
  - `cmd`: the grammar, the diagnostics, both producers, the template and
    the language server;
  - `authguard` and `openapi`, requiring the new root.
- `routes/v1` files written before stay valid. `gen` rewrites them without
  `entity`.
- Confab drops its three `//h:entity` lines. Its events and locales get
  sources (`events.search`, `locales.search`), `speakers.search` gets a
  user, and `translations/{locale}` stays `//h:search` with a source for
  `locale`.
- `ui/palette` (ui A10) builds on `Kind`, `Bits`, `From`, `Table.Link` and
  `Hit.Link`.

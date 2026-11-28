# routes/v1: the rules Check enforces

docs/interface.md §8 makes these rules part of the contract.

`internal/routesv1.Check(v any) error` validates one decoded routes/v1 document
(`json.Unmarshal` into `any`). It returns every problem joined with
`errors.Join`, each as `<location>: <message>`, in document order. Locations use
the JSON path: `domain`, `package.name`, `routes[2].params[0].kind`,
`permissions[1].var`. Messages are lowercase, no final period.

## Shape (also in schema/routes.v1.json)

- Document: object. Required: domain, producer, source, package, surface, prefix, routes, permissions.
  Unknown extra fields are allowed everywhere (a later minor version may add them).
- `domain`: string, exactly "routes/v1"; another `routes/vN` is `want "routes/v1", got "routes/v2"`.
- `producer`: non-empty string.
- `source`: non-empty string: the handler package's directory (html) or the spec's path
  (json), relative to the module root, so `handlers vet` can pair a package with its file.
- `package`: object with `name` (a Go identifier, not `_`) and `dir` (non-empty, slash-separated,
  relative, clean: `.` or no `.`/`..` elements, no leading or trailing `/`, no `//`).
- `surface`: "html" or "json". Anything else: `unknown surface "x"` (fail closed).
- `prefix`: "" or starts with "/" and doesn't end with "/", no "//", no "{".
- `routes`: array of route objects (may be empty). `permissions`: array (may be empty).

Route: required key, method, pattern, name, var, text, auth, view, search, params.
- `key`: dot-separated segments, each `[a-z0-9][A-Za-z0-9_]*`.
- `method`: GET POST PUT PATCH DELETE (upper case only).
- `pattern`: starts with "/"; doesn't end with "/" unless it is exactly "/"; no "//".
  Wildcards are `{name}` or `{name...}` with name a Go identifier; a wildcard fills a whole
  segment; `{name...}` only as the last segment; no wildcard name twice; no `{$}` (the
  generator writes it).
- `name`: an exported Go identifier. `var`: a Go identifier starting with `L10n`.
- `text`: non-empty. `auth`: public, session or permission; anything else: `unknown auth "x"`.
- `permission`: required and non-empty when auth is permission; must be absent when auth is
  public or session.
- `view`, `search`: booleans. `entity`: optional non-empty string; (A9) no longer written, and generators ignore it. `maxBody`: optional
  integer, at least 1; must be absent when method is GET. `operation`: optional
  non-empty string; required when surface is json, absent when html.
- `params`: array, in signature order. Each param requires name, go, var, type, import,
  parse, kind, label: `name` and `go` Go identifiers; `var` a Go identifier starting with
  `L10n`; `type` non-empty; `import` a string ("" for none); `alias` present exactly when
  `import` isn't "", a Go identifier, and the qualifier `type` starts with (`shop.OrderID`
  with alias `shop`); `parse` one of string bool text int int8 int16 int32 int64 uint
  uint8 uint16 uint32 uint64 (the underlying type's width); `kind` segment or rest; `label` non-empty.
  (A9) Optional `from`: a non-empty string, the key of the param's source.

Permission: required key, level. Optional `from` (a package name: a Go identifier, not `_`, not the document's own package name). Without `from`, required `var` (a Go identifier starting with `Perm`) and `text` (non-empty). With `from`, `var` and `text` must be absent; `level` verified or unverified.

## Rules across fields

1. Route keys are unique: `routes[3].key: "a.b" also at routes[1]`.
2. Method and pattern pairs are unique: `routes[3].pattern: GET /a also at routes[1]`.
3. Route names are unique.
4. Every `var` (routes' and params') is unique in the document, and no var is `L10n` or `l10n`.
5. Within a route, param names are unique, and the pattern's wildcards and the params match
   one to one by name: `routes[0].params[1]: no wildcard {sid} in /a/{id}` and
   `routes[0].pattern: wildcard {x} has no param`.
6. A param's kind is rest exactly when its wildcard is `{name...}`.
7. `entity` names a route key in the document. (A9) A param's `from` names a route of the
   document other than its own, whose method is GET and which has no params:
   `routes[1].params[0].from: unknown route key "x"`, `"x" is the param's own route`,
   `"x" is POST, not GET`, `"x" has params`.
8. Permission keys and vars are unique.
9. Every route's `permission` names an entry of `permissions`; every entry is named by at
   least one route (`permissions[2]: no route uses "x"`).
10. Fail closed: an unknown auth, surface, parse, kind or level is an error, never a default.
11. A route's `name` becomes a package-level identifier of the generated package, so it
    is none of the names that package declares (`Handler`, `Register`, `Mount`, `Routes`,
    `ByKey`, `Prefix`, `L10n`) and no `var` of the document: `routes[0].name: "Register" is also a name the
    generated package declares`, `routes[0].name: "PermOrdersManage" is also permissions[0].var`.

12. An `alias` names one import path, and an import path has one alias:
    `routes[1].params[0].alias: "shop" also names "example.com/shop/internal/shop"`,
    `routes[1].params[0].alias: "example.com/shop/internal/shop" is already imported as "shop"`.

13. A permission entry with `from` has no `var` or `text`, and `from` does not name the document's own package:
    `permissions[0].var: must be absent with from`,
    `permissions[0].text: must be absent with from`,
    `permissions[0].from: "adminroutes" is the document's own package name`.

## Conformance files

`testdata/routes/valid/*.json`: minimal (no routes, no permissions), full html (every optional
field, a rest wildcard, shared permission, entity (no longer written, A9), a param's `from`, root pattern "/", unknown extra fields),
json surface with operations, prefix "" and dir ".".

`testdata/routes/invalid/*.json`: each breaks exactly one rule, with a sibling `.err`
holding Check's exact message (the whole joined error text). One per shape rule that
matters and one per rule across fields above, plus: not an object, domain missing, domain
v2, unknown auth, unknown surface, permission on a public route, missing permission on a
permission route, operation on an html route.

`testdata/routes/README.md` says what the files are, as genx's testdata/labels/README.md does.

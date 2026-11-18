# routes/v1: the rules Check enforces

The draft for the freeze (roadmap M1.5); interface.md will take these rules over.

`internal/routesv1.Check(v any) error` validates one decoded routes/v1 document
(`json.Unmarshal` into `any`). It returns every problem joined with
`errors.Join`, each as `<location>: <message>`, in document order. Locations use
the JSON path: `domain`, `package.name`, `routes[2].params[0].kind`,
`permissions[1].var`. Messages are lowercase, no final period.

## Shape (also in schema/routes.v1.json)

- Document: object. Required: domain, producer, package, surface, prefix, routes, permissions.
  Unknown extra fields are allowed everywhere (a later minor version may add them).
- `domain`: string, exactly "routes/v1"; another `routes/vN` is `want "routes/v1", got "routes/v2"`.
- `producer`: non-empty string.
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
- `view`, `search`: booleans. `entity`: optional non-empty string. `operation`: optional
  non-empty string; required when surface is json, absent when html.
- `params`: array, in signature order. Each param requires name, go, var, type, import,
  parse, kind, label: `name` and `go` Go identifiers; `var` a Go identifier starting with
  `L10n`; `type` non-empty; `import` a string ("" for none); `parse` one of string int
  uint bool text; `kind` segment or rest; `label` non-empty.

Permission: required key, var, text, level. `key` as a route key; `var` a Go identifier
starting with `Perm`; `text` non-empty; `level` verified or unverified.

## Rules across fields

1. Route keys are unique: `routes[3].key: "a.b" also at routes[1]`.
2. Method and pattern pairs are unique: `routes[3].pattern: GET /a also at routes[1]`.
3. Route names are unique.
4. Every `var` (routes' and params') is unique in the document, and no var is `L10n` or `l10n`.
5. Within a route, param names are unique, and the pattern's wildcards and the params match
   one to one by name: `routes[0].params[1]: no wildcard {sid} in /a/{id}` and
   `routes[0].pattern: wildcard {x} has no param`.
6. A param's kind is rest exactly when its wildcard is `{name...}`.
7. `entity` names a route key in the document.
8. Permission keys and vars are unique.
9. Every route's `permission` names an entry of `permissions`; every entry is named by at
   least one route (`permissions[2]: no route uses "x"`).
10. Fail closed: an unknown auth, surface, parse, kind or level is an error, never a default.
11. A route's `name` becomes a package-level identifier of the generated package, so it
    is none of the names that package declares (`Handler`, `Register`, `Routes`, `ByKey`,
    `L10n`) and no `var` of the document: `routes[0].name: "Register" is also a name the
    generated package declares`, `routes[0].name: "PermOrdersManage" is also permissions[0].var`.

## Conformance files

`testdata/routes/valid/*.json`: minimal (no routes, no permissions), full html (every optional
field, a rest wildcard, shared permission, entity, root pattern "/", unknown extra fields),
json surface with operations, prefix "" and dir ".".

`testdata/routes/invalid/*.json`: each breaks exactly one rule, with a sibling `.err`
holding Check's exact message (the whole joined error text). One per shape rule that
matters and one per rule across fields above, plus: not an object, domain missing, domain
v2, unknown auth, unknown surface, permission on a public route, missing permission on a
permission route, operation on an html route.

`testdata/routes/README.md` says what the files are, as genx's testdata/labels/README.md does.

# ADR 0002: Spike S1, zp's admin routes against the draft rules

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-18 |
| Roadmap item | M1.1 |
| Deciders | Claude (the measurements and the clarifications); the grammar is epistemer's at the freeze (M1.5) |

## Context

Brief §5 drafts the directive grammar and the engine's diagnostics. Before
the freeze, every admin route zp declares had to be run through those rules,
to learn whether the grammar can express them, how many would fail which
check, what converting Echo patterns to `ServeMux` patterns costs, and how
many permissions per-route permissions would make (open question Q1).

Method: a throwaway program in a scratch folder parsed a copy of zp's admin
handler package with `go/parser`, read every comment of every file, applied
the draft rules, converted `:name` wildcards to `{name}`, registered every
pattern under the mount prefix into a scratch `http.ServeMux`, and recovered
its panics. Paths without a leading slash were counted separately, since
concatenating them onto the prefix hides them from `ServeMux`. Only counts
leave the scratch folder.

## Measurements

19 files (17 with directives), **217 routes**: 109 `GET`, 108 `POST`.
Parameters: 243 `int64` and 12 `string`, all supported by the draft binder.
No Echo `*` wildcards. 18 paths have a capital letter in a static segment
(`/fees/copyFrom`); no path other than the root ends with `/`.

| What the draft rules report | Routes |
|---|---|
| A `:name` wildcard converted to `{name}` | 156 |
| The root pattern, which needs `{$}` | 1 |
| **Error:** duplicate key, also a duplicate method and pattern (one pair; `ServeMux` reports it as a conflict) | 2 |
| **Error:** near-miss `//p[...]` directive | 3 directives |
| **Error:** a wildcard whose name differs from its parameter's (binding by name gave the parameter its zero value) | 3 |
| **Error (new rule, below):** a path without a leading `/`, so concatenating it onto the prefix gave a different path than meant | 4 |
| **Error under the draft:** a flag directive with the value `true` (`view` 12, `search` 10) | 22 |
| **Error under the draft:** a key segment in camelCase (`pricingPlans`) | 84 |
| Unknown directives, detached blocks, a directive twice, an unknown `entity`, a pattern `ServeMux` refuses other than the duplicate | 0 |
| **Warning:** missing `desc` (all omitted; none empty) | 63 |
| `auth` | 207 default, 10 `public`; no other level is used |

The faults overlap: two of the routes with a mismatched wildcard also lack
their slash, so **7 distinct route declarations** carry a fault (the
duplicate pair, one mismatch only, two slashless only, two with both).

Permissions: 207 guarded routes, but one is the duplicate, so **206
permission keys**. 43 are page/action pairs by key (`….create`/`….store`,
`….edit`/`….update`): 163 keys if every pair shares one.

## Options

For each draft rule that the census contradicts:

1. **Key grammar.** Keep snake case only, and 84 of 217 keys would have to
   change, though keys are permanent; or allow camelCase segments, which is a
   common style. Producers derive Go names from keys, so `pricingPlans` and
   `pricing_plans` would give one Go name: allowing both needs a collision
   check.
2. **Flag values.** Refuse any value, and 22 directives fail for writing
   what they mean; accept `true` as well as no value; or also accept
   `false`. zp's real fault was that *any* value meant true. `false` was never
   written, means the same as leaving the directive out, and is a trap for a
   reader who checks only that the directive is present.
3. **Leading and trailing slashes.** The draft didn't say. Four zp paths
   lack a leading slash, and each answers at a path nobody meant. A trailing
   slash on a path other than the root makes `ServeMux` match the whole
   subtree; zp has none.
4. **Q1, pairing pages and actions.** Pair by key suffix as a default, which
   is an implicit naming convention that a renamed key silently breaks; or
   keep one permission per route with `//h:perm` to pair them explicitly:
   206 permissions, or 163 if every pair shares one.
5. **Capitals in static segments.** `ServeMux` is case-sensitive, and URL
   builders always write the declared case, so they work; refusing them
   would change 18 paths for no fault.

## Decision

Recommended for the freeze, applied to the brief now as clarifications:

1. A key segment matches `[a-z0-9][A-Za-z0-9_]*`: snake case and camelCase
   both work. Two keys of one package whose Go names come out the same are an
   error.
2. `view` and `search` take no value or `true`; any other value, `false`
   included, is an error.
3. A path starts with `/`, and only the root ends with one. The root is
   written `/`; the generator registers it as `{$}` under the prefix. The
   prefix starts with `/` and doesn't end with one, and a pattern is the
   prefix followed by the path.
4. Q1 keeps its default: one permission per route, paired only by an
   explicit `//h:perm`. How a grant screen groups them is the UI's business.
5. Capitals in static segments are allowed.

Every zp route is expressible: after the mechanical conversions, each one
compiles to a `ServeMux` pattern, and the only conflict is the duplicate the
rules exist to catch. The kill criterion of M1.1 isn't met.

## Consequences

- The rules catch four kinds of real fault in zp's routes, in 7 declarations:
  a duplicate, dead parameter directives, three bindings by a mismatched
  name, and four paths that lack their slash. The fixture of M2.1 rebuilds
  each in a toy package.
- Brief §5.2 and §5.3 change as above. The grammar is still epistemer's to
  freeze.
- M5.2's comparison with zp has to account for these 7 declarations, whose
  generated paths or bindings will differ from zp's on purpose.
- [Review 0002](../reviews/0002-s1-census.md) checked this record.

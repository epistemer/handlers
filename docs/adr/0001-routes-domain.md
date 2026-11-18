# ADR 0001: Routes as one domain from two producers

| | |
|---|---|
| Status | Accepted |
| Date | 2026-11-18 |
| Roadmap item | M0.2 |
| Deciders | epistemer (D1–D5, D11, D16, D18, D19; D6, D7 and D10 with the plan); proposed by Claude |

## Context

An app built on neozp serves two kinds of routes: admin pages, rendered on
the server and declared next to their handlers, and a JSON API that other
programs call, described by an OpenAPI file. zp had a pipeline for each, and
they shared nothing but the server. The admin pipeline's comment directives
are liked by the people who write them, but its faults passed silently: a
duplicate key, directives that gofmt broke, values it ignored, permissions it
stored and never enforced. The API's spec was copied four times and the
copies drifted. [brief.md](../brief.md) §1 lists the failure shapes.

What constrains the answer:

- genx is neozp's generation driver: producers write versioned domain
  files, trivial generators (jq and a template) turn them into code, and a
  trivial generator may not parse source or derive Go names from a neozp
  producer's data (genx ADR 0001 §7).
- Every text a generator makes visible gets an i18n key through a
  `labels/v1` file. Permissions are the exception: `access.Permission`
  registers `permission.<key>` itself, and declaring through it is how auth
  declares permissions.
- auth's middleware is plain `net/http` (auth D21); permissions are declared
  with `access.Permission` and checked with `access.Can`.
- errors maps an error to a status through `KindOf` and to a message
  through `Public`.

## Options

1. **Two pipelines, as zp.** Admin directives produce Go; the API keeps a
   third-party server. Registration, guards, labels and search are written
   twice, and the API gets none of the admin side's labels or permissions.
2. **One schema for both.** Either a Go DSL for every route, with the
   OpenAPI file generated from it, or OpenAPI for admin pages too. The first
   needs a JSON Schema DSL of our own for request bodies; the second makes
   admin pages three times longer to declare and gives HTML responses a
   format that says little about them. epistemer chose against both (D2, D3).
3. **One domain from two producers.** Directives and the spec stay where they
   are, and each producer writes the same facts as `routes/v1`. The trivial
   generators `routes`, `labels` and `permissions` read that domain, so the
   two surfaces get one registration shape, one guard, one label scheme and
   one search.

## Decision

Option 3. `handlers gen` reads the directives of a handler package and
`handlers openapi` reads a spec; both write `routes/v1`, which this
repository owns (schema and conformance files) until another package uses it.
A single rule engine checks directives for every front end: the producer,
`handlers vet`, `handlers lsp`, and the hooks in `dev/`. The decisions table
of [brief.md](../brief.md) §3 is part of this record. D6, D7 and D10 are
settled by it; the other rows marked (Claude) stay open until the interface
is frozen (M1.5).

## Consequences

- A route's facts are data. Search, permissions and any later nav or
  breadcrumbs read the generated table; no route detail travels as i18n
  meta, so genx needs no `meta` on `labels/v1` (D10). genx's backlog item for
  that meta is moot.
- `routes/v1` is the second neozp domain after `labels/v1`. Its schema and
  conformance files are M1.4; moving it to genx waits for a second user.
- The API's typed server still comes from a third-party generator, chosen by
  spike S2 (M1.2, ADR 0003). If none fits without patching generated code,
  `routes` renders the API too.
- The rule engine is the only place a directive is checked. The VS Code
  extension stops checking anything itself (D19).
- Route keys become permanent, because each holds translations and
  permission grants (D16).
- Rendering, nav menus and CRUD tables stay with later UI packages (D17).

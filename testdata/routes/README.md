# routes/v1 Conformance Files

These files are the conformance test suite for `routes/v1` documents specified in `internal/routesv1/rules.md` and `schema/routes.v1.json`.

- `valid/*.json`: Valid `routes/v1` documents demonstrating minimal, full HTML with every optional field (rest wildcard, shared permission, entity (no longer written, A9), a param's `from`, root pattern "/", unknown extra fields), JSON surface with operations, root prefix "" with package dir ".", and taken permissions with `from`.
- `invalid/*.json`: Documents violating exactly one validation rule, each paired with a sibling `.err` file containing the exact error message from `internal/routesv1.Check`.

Producers (such as handlers gen and handlers openapi) and generators use these conformance files to verify agreement with the rules engine.

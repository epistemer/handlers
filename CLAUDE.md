# handlers

neozp's route package: admin routes from comment directives, API routes from
an OpenAPI file, one `routes/v1` domain, and from it registration on
`net/http`, labels and `auth/access` permissions. Tools for editors and agents
live in `dev/`.

- Read [docs/roadmap.md](docs/roadmap.md), then the newest entry of
  [docs/status.md](docs/status.md). The design is
  [docs/brief.md](docs/brief.md) until ADR 0001 replaces it; the contract will
  be `docs/interface.md` (M1.5).
- Claude owns this repository until v1.0.0, on the terms of
  [docs/ownership.md](docs/ownership.md) (delegated 2026-11-18). Only API
  design is epistemer's, and the freeze of interface.md.
- Load the `genx`, `i18n`, `errors` and `auth` skills before touching
  generators, labels, errors or permissions.
- Every diagnostic the rule engine gives has a position and a stable message,
  tested word for word. The producer, `handlers vet` and `handlers lsp` share
  the engine: never check a directive anywhere else.
- Producers resolve every Go name and hand templates finished literals; the
  trivial generators only render (genx ADR 0001 §7).
- zp (`/home/epistemer/zp`) is private and read-only. Keep its names, routes
  and spec out of this repository, which is public: examples and tests use a
  toy shop. zp's own issues are out of scope.
- Every working session adds an entry to docs/status.md.

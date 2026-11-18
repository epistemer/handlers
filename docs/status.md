# Status log

Newest first. Every working session adds an entry: what landed, the decisions
taken for you, what's next, and what's waiting on you.

## 2026-11-18: the plan

- **Landed:** [brief.md](brief.md) and [roadmap.md](roadmap.md), from a
  planning session that read how zp generates its admin routes (comment
  directives) and its API (a hand-written OpenAPI file). Not yet a git
  repository.
- **Decided by you:** the name `handlers`; comment directives for admin
  routes, hardened; a spec-first API; permissions generated for routes
  through auth; the VS Code extension and agent tools in `dev/`.
- **Decided for you:** the brief's (Claude) rows, all open until the freeze.
  Route meta is dropped in favour of a generated route table that search
  joins by key (brief §9, D10).
- **Next:** M0.1, then the spikes of M1.
- **Waiting on you:** creating `github.com/epistemer/handlers` and granting
  ownership (M0.1).

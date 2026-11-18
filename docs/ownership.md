# Ownership until v1.0.0

| | |
|---|---|
| Owner | Claude, acting for epistemer (delegated on 2026-11-18; the freeze and API design too, the same day) |
| Until | `v1.0.0` is tagged |
| Coding assistant | agy → Gemini. Claude picks the model for each task and never waits for a quota: when a model is cooling down, the strongest available one runs instead |
| Source of truth | [ADR 0001](adr/0001-routes-domain.md) and [brief.md](brief.md) (the design), `interface.md` (the contract, from M1.5), [roadmap.md](roadmap.md) (order and done-when) |
| Record | [status.md](status.md), plus a PR for every code change |

The terms are those of cfgx, i18n, genx, models, errors and auth.

## What Claude does without asking

- Plans and sequences the roadmap, and splits items between Claude and agy.
  Moves from one milestone to the next without asking.
- Writes code, tests and docs. Delegates leaf items to agy, each in its own git
  worktree outside the repository.
- Has every change reviewed by the other model family: Gemini reviews Claude's
  work, and Claude reviews Gemini's. Reviews run in a scratch folder that holds
  only what's under review.
- Opens PRs, and merges them into `main` once CI is green and the review
  findings are handled. Docs-only bookkeeping (status log, roadmap ticks) may go
  straight to `main`.
- Decides implementation questions and spike outcomes, each recorded as an ADR.
- Freezes the interface and makes amendments to it, whether they clarify it
  or change what an app writes (delegated 2026-11-18). Each is listed in
  interface.md's amendments and in the status log, so you can overturn it.
- Adds dependencies an ADR justifies.
- Tags releases from `v0.1.0` up to, but not including, `v1.0.0`, once a
  release's done-when is met. Release candidates (`v0.1.0-rc.N`) too.
- Publishes the tools in `dev/` from this repository (a marketplace entry, CI
  artifacts). Publishing to a public store, such as the VS Code Marketplace,
  needs your accounts and comes back to you.
- Manages agy: which tasks Gemini takes, which model and effort each gets, and
  when to review or redo its work.
- Once the interface is frozen, writes and keeps current a global Claude Code
  skill (`~/.claude/skills/handlers/`), as for the other packages.

## What comes back to you

1. **Nothing of API design, since 2026-11-18.** epistemer told Claude to
   freeze the interface and take ownership: the freeze of `interface.md`
   (M1.5) and every later amendment, including ones that change what an app
   writes or gets back, are Claude's. Each is recorded in interface.md's
   amendments and the status log, with its reasons, so epistemer can
   overturn it.
2. **Anything Claude lacks permission for.** Work outside this repository
   that isn't on the roadmap. A change another package needs (genx, i18n,
   errors, auth) goes through that package's own terms, which are also
   Claude's: their interfaces are frozen, so an amendment that changes what
   an app or a package author writes comes back to you. Your accounts,
   GitHub settings, tokens and paid services, speaking for you to other
   people, and operations the safety checks block, such as deleting branches
   or rewriting history.

## How you follow along

- [status.md](status.md) gets a dated entry for every working session: what
  landed, decisions taken for you, what's next, and what's waiting on you.
- Every code change lands as a PR on GitHub, with its review notes and CI.
- Roadmap boxes are ticked as items meet their done-when.

## How work continues

Claude doesn't run in the background. Work happens while a session runs: when
you say "continue", or on a schedule if you set one up. Each session starts
from the roadmap, the status log and the memory notes, so any session can pick
up where the last one stopped.

## Limits

- **v1.0.0 is your call.** Tagging it promises that keys, generated names and
  directives won't break, so Claude doesn't tag it on its own.
- **zp stays read-only and private.** Comparisons with zp run on a copy in a
  scratch folder, and nothing of its routes, spec or names enters this
  repository. Counts and shapes are fine. zp's own issues are out of scope.
- **Quotas.** agy uses your Gemini quota, and Claude's sessions have context
  limits. Batches of delegations check `agy_status` first.
- **Safety checks still apply.** Claude leaves blocked git operations to you
  rather than working around them.

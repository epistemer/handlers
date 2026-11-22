# ADR 0005: URL builders return a link, and a gate adapts a guard for ui

| | |
|---|---|
| Status | Proposed, for epistemer (ui's roadmap D7.3) |
| Date | 2026-11-22 |
| Roadmap item | ui D7.3; handlers v0.2.0 |
| Deciders | epistemer decides; Claude proposes, from ui's design of actions and tables (ui's server-state-design.md §4, §5.3, §10; its ADRs 0008 and 0009) and the spike that tried it (§12.1) |

## Context

ui's actions are shown only when the request's guard admits the route
they go to (ui ADR 0009, decided by epistemer as S1). So whatever binds an
action to a route must carry both the route and the built URL. Today a
generated builder returns a `string`, and finding a route takes its key:
an action would name its route twice, and nothing would check the two
agree.

ui decided its side on 2026-11-22: every element and helper that takes an
address takes `ui.Link`, an alias of `interface{ URL() string }`
(ui's amendment A5, built and released in ui v0.2.0). It asks handlers for
three things (ui's design §10):

- builders that return a link with the route it goes to;
- the link's method, so an action knows a POST from a GET;
- an adapter from a guard's `Admit` check to ui's gate,
  `func(interface{ URL() string }) bool`, which ui owns the type of, so
  neither module imports the other (ui's S7).

## Options

1. **A second builder per route** (`CategoryLink(id)`), keeping the string
   one. Nothing breaks; every route has two names.
2. **Builders return `handlers.Link`** (this proposal). One name per
   route. Every call that used the string writes `.URL()`; ui's elements
   take the link as it is.
3. **No change; ui matches URLs to routes at render.** A lookup per action
   per row, and a mismatch found only when a page renders.

## Decision (proposed)

Option 2.

```go
package handlers

// Link is a route's URL, built by a generated builder: the route it goes
// to and the address with its parameters written in.
type Link struct{ /* unexported: the route and the address */ }

func (l Link) URL() string    // the address: Prefix + the path, and a query when With added one
func (l Link) Method() string // the route's method: GET, POST, …
func (l Link) Route() Route
func (l Link) String() string // URL(), for fmt and logs

// With returns the link with q as its query (replacing any), for a page's
// own parameters: adminroutes.Orders().With(url.Values{"status": {"paid"}}).
func (l Link) With(q url.Values) Link

// Gate adapts a guard's check to ui's actions.Gate: a Link asks admit
// about its route; any other link (ui.URL, an external address) has no
// route to ask about, and is admitted.
func Gate(admit func(Route) bool) func(interface{ URL() string }) bool
```

Generated builders keep their names and parameters and change their result:

```go
func Category(id int64) handlers.Link // URL() "/admin/categories/42"
```

An app sets ui's gate once per request from its guard:

```go
admit, err := guard.Admit(r)
if err != nil { … }
ctx := actions.Gate.With(r.Context(), handlers.Gate(admit))
```

## Consequences

- **Breaking**, so handlers v0.2.0, with `authguard`, `openapi` and `cmd`
  following. Every use of a builder as a string writes `.URL()`:
  `http.Redirect(w, r, adminroutes.Category(id).URL(), …)`. A `%s` in fmt
  keeps working through `String()`. ui's elements take the link with no
  call.
- `Link` is comparable and cheap: a route (already in the table) and a
  string.
- The gate never decides what is allowed: the route's guard still runs on
  every request. It only lets a page stop offering what would be refused.
- `Table.Search` (§14) is unchanged.
- handlers still imports no ui package, and ui none of handlers.

## Questions for epistemer, each with a default

| # | Question | Default |
|---|---|---|
| H1 | The type's name | `handlers.Link` |
| H2 | Adding a query | `With(url.Values)`, replacing any query; the alternative is `Query(key, value string)` adding one parameter |
| H3 | A link that isn't a route in `Gate` | admitted: nothing to ask about |
| H4 | Keep a string builder beside the link (`CategoryURL`) for a migration period | no: one name per route; v0's rules allow the break |

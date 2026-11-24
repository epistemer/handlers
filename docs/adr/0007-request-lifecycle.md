# ADR 0007: the request's lifecycle, and permissions shared across packages

| | |
|---|---|
| Status | Locked by epistemer on 2026-11-24 with the defaults of N1–N11. Amendment A7, for v0.4.0. Reviewed: [0008](../reviews/0008-request-lifecycle.md) |
| Date | 2026-11-24 |
| Roadmap item | Confab iteration 1: F16, F25 (with ui), F21, F22; neozp decisions D4 and D6 |
| Deciders | epistemer decides (the decisions of 2026-11-24 settle the what; the questions N1–N11 below are the how). Claude proposes |

## Context

Confab, the example app on every neozp package, wrote three things every
app would copy (its findings, 2026-11-23):

- **Recovery** (F16). A panic in a handler reaches `net/http`, which logs
  it and drops the connection. With F25, Chrome then sent a committed POST
  again. Confab wraps every route in its own `Recover`.
- **A flash** (F25). ui's `Reply.Go` can't carry a toast across a redirect,
  so Confab keeps a key in a cookie (`flash.go`) and reads it in its shell.
- **Permissions twice** (F22, F21). The admin and the API can't share a
  permission, so a reviewer is granted `talks.review` and
  `api.talks.review`, and shared permissions read "Talks Review".

epistemer decided on 2026-11-24 (neozp `docs/decisions.md`):

- **D4.** A handlers subpackage, `handlers/middleware`, owns the request's
  lifecycle: recovery, used by default by `Register` and `Mount`; a flash,
  a level and an i18n key carried across a redirect and rendered in the
  reader's language; redirect, htmx-aware redirect and error-page helpers;
  later, request ids and access logs. ui shows flashes as toasts through an
  interface ui declares, so ui doesn't import handlers. This reverses the
  brief's D17 for flash and htmx.
- **D6 (H3).** A route may name a permission another routes package of the
  same app declares. Declaring it twice stays an error.
- **H4.** A permission can carry its text after the key; without it, the
  rule of §13.2 stands.

zp's admin handler and its flash helpers are the record of what an admin
needs here: render a page with a status, redirect with and without htmx,
answer 404 and 500 as pages, and flash success, warning and error messages
across a redirect. Nothing of their code or names is copied.

## Decision

### 1. `handlers/middleware`, in the root module

A new package in `github.com/epistemer/handlers`, depending on errors, i18n
and the standard library only, as the core does (D15).

```go
package middleware // import "github.com/epistemer/handlers/middleware"
```

**Import direction (N3).** The core can't import `middleware` if
`middleware` imports the core (for `ErrorWriter`, `Status`, `WriteText`),
and `NewMount` must recover by default. So recovery is implemented once in
an internal package both use, and the error it produces is declared in the
core next to `ErrNotFound` and its siblings. `middleware.Recover` is the
same recovery for an app's own handlers outside a routes package.

### 2. Recovery, on by default (N2)

```go
package handlers

// ErrPanic is the error a recovered panic becomes: 500, for people "the
// server failed to answer". Its cause is a *Panic.
var ErrPanic = errors.Key("errors.handlers.panic", "the server failed to answer", errors.Internal)

// Panic is a recovered panic's value and the stack where it happened.
type Panic struct {
	Value any
	Stack []byte
}

func (p *Panic) Error() string // "panic: <Value>"
```

```go
package middleware

// Recover recovers a panic in next as registration does (§12.1, step 0),
// writing ErrPanic with ew. For the app's own handlers; routes registered
// by Register and Mount already recover.
func Recover(ew handlers.ErrorWriter) func(http.Handler) http.Handler
```

§12.1 gains a step 0, outermost, so it covers the body limit, the guard,
`WithMiddleware`, validation, binding, the handler and the catch-all:

- A panic is logged once, with `slog`'s default logger at error level:
  `handlers: panic`, with `route` (the key, passed by registration, since
  step 0 runs before the route is in the context), `panic` and `stack`.
  `middleware.Recover` logs `method` and `path` in place of `route`.
- `http.ErrAbortHandler` is panicked again, as `net/http` expects.
- If nothing has been written yet, `ErrPanic.Wrap(&Panic{…})` goes to the
  route's error writer, so the person sees the app's 500 page and the app
  can report the `*Panic` (`errors.As`). The default writers don't log
  `ErrPanic` again.
- If the response has started, it can't become a page: the connection is
  aborted (`panic(http.ErrAbortHandler)`), so the client sees a truncated
  response rather than a complete-looking one.
- If the error writer itself panics (a render panic was Confab's case),
  that is logged too; if nothing was written yet, `WriteText` answers; otherwise, or if `WriteText` panics,
  the connection is aborted.
- Before the error writer or `WriteText` answers, the headers are put
  back as they were before the route ran (built: review of #25), so an
  outer middleware's headers stay and the handler's go.
- The recovering writer has `Unwrap`, `Flush` and `Hijack` (a flush or a
  hijack counts as started), so `http.ResponseController` and streaming
  keep working.
- `Panic.Error` formats the value defensively: a value whose `String` or
  `Error` panics prints as its type.

There is no opt-out: an app that reports panics elsewhere does it in its
error writer, from the `*Panic`. A test with `httptest.NewRecorder` sees a
500 and the log line (or, after a started response, the
`http.ErrAbortHandler` panic, with the original stack in the log).

### 3. Redirects, htmx and the error page

```go
// HTMX reports whether htmx sent r (HX-Request: true).
func HTMX(r *http.Request) bool

// Boosted reports whether a boosted link or form sent r (HX-Boosted: true).
func Boosted(r *http.Request) bool

// Redirect sends the browser to to: with htmx, 204 and HX-Redirect, so htmx
// loads it as a whole page; without, 303 See Other. An address that isn't
// this site's (not one leading slash, a scheme or a host, a backslash or a
// control character) returns ErrNotLocal and writes nothing. A nil to panics.
func Redirect(w http.ResponseWriter, r *http.Request, to interface{ URL() string }) error

// ErrNotLocal is Redirect given another site's address, such as a return
// address from a query: 400, since it comes from the request.
var ErrNotLocal = errors.Key("errors.handlers.not_local", "the address isn't this site's", errors.Invalid)

// ErrorPage returns an error writer for handlers.WithErrors. It logs err as
// WriteText does (status 500 or more, ErrPanic excepted), then calls page
// with handlers.Status(err); page writes the whole answer, status
// included. If page returns an error, that is logged too; if nothing was
// written, err is written as WriteText writes it, without logging it
// again; if something was, the connection is aborted.
func ErrorPage(page func(w http.ResponseWriter, r *http.Request, status int, err error) error) handlers.ErrorWriter
```

- `Redirect` takes `interface{ URL() string }`, as every ui helper that
  takes an href does (ui amendment A5): a generated builder's `Link`, or a
  `ui.URL` for a return address read from a query.
- A 404 or 500 page is still an error a handler returns
  (`handlers.ErrNotFound`, or any error with its kind); `ErrorPage` is the
  writer that renders it. What a page looks like with htmx (Confab shows a
  toast and swaps nothing) stays the app's and ui's: `page` asks `HTMX(r)`.
- An answer that changes the page in place (ui's `Reply.Else`) is ui's.

### 4. The flash (N4–N7)

```go
const (
	LevelInfo    = "info"
	LevelSuccess = "success"
	LevelWarning = "warning"
	LevelError   = "error"
)

type Flash struct{ /* unexported */ }

func NewFlash(opts ...FlashOption) *Flash

type FlashOption func(*flashOptions)

func FlashName(name string) FlashOption // the cookie's base name; default "flash"
func FlashInsecure() FlashOption        // no Secure and no __Host- prefix, for development over plain HTTP

// Middleware reads the request's flash cookie into the context and writes
// it back with the response's header.
func (f *Flash) Middleware(next http.Handler) http.Handler

// Add carries msg to the next page (see below). It panics on a level other
// than the four, and outside f.Middleware.
func (f *Flash) Add(ctx context.Context, level string, msg i18n.Ter, args ...any)

// Messages returns the messages this request delivers, oldest first, with
// their levels. It returns nothing outside f.Middleware, and nothing to an
// htmx request that isn't boosted.
func (f *Flash) Messages(ctx context.Context) iter.Seq2[string, i18n.Ter]
```

An app wires it once per HTML surface, inside i18n's middleware (§3.3's
wiring, with the flash around the admin mux):

```go
fl := middleware.NewFlash()
root.Handle(adminroutes.Prefix+"/", a.Load(auth.Cookie())(fl.Middleware(admin)))
srv := &http.Server{Handler: tr.Middleware()(root)}

// in a handler
fl.Add(r.Context(), middleware.LevelSuccess, L10nEventSaved, ev.Name)
return middleware.Redirect(w, r, adminroutes.Event(ev.ID))
```

Wrap the HTML surfaces with it, not the API or static files: only an HTML
page delivers a flash (below), but every response a flash middleware sees
can carry one on.

**ui's side (N4).** Every type in the two methods is the standard library's
or i18n's, and the level is a plain `string`, so ui declares

```go
type Flasher interface {
	Add(ctx context.Context, level string, msg i18n.Ter, args ...any)
	Messages(ctx context.Context) iter.Seq2[string, i18n.Ter]
}
```

and `*middleware.Flash` satisfies it with neither module importing the
other (checked in a scratch module). ui's `Reply` hands its toast to `Add`
when it redirects, and its toast region reads `Messages(ctx)`, which needs
no `ResponseWriter`. ui maps its tones to the four levels.

**What travels (N5).** The flash stores what renders in the reader's
language whenever it can:

- `msg` that is a key (an `i18n.Ker`, not an `error`) travels as its key
  and args, and renders with `i18n.DynamicWrap(key)` in the locale of the
  request that shows it: a flash set just before a language change reads
  in the new language.
- An `error`, or any other `Ter` (a `Static`, ui's refusal text), is
  rendered when it is added, in the writer's locale: an error's arguments
  are inside it (errors v0.1.0 doesn't expose them), so its key alone
  would render as "%s is not valid".
- Args that survive, by their kind, so a defined type such as `EventID`
  keeps `%d`: strings, bools, integers and floats; and a key that isn't an
  `error` (rendered in the reader's locale, as i18n renders a label
  argument). An `error` argument is rendered when added with `errors.T`,
  another `Ter` with its `T`, both in the writer's locale; anything else
  with `fmt.Sprint`. Errors are tested before keys, for `msg` and args
  alike, since a declared error is also a key.

**When a message is delivered (N6).** A flash is for the next page:

1. The middleware reads the cookie when the request arrives (a cookie over
   4 KiB is ignored undecoded). Its messages are this request's, unless the
   request is a `HEAD` or an htmx request that isn't boosted: a fragment
   rarely shows toasts, so they wait.
2. `Messages` reads them from the context, any number of times. Messages
   added during this request aren't among them: to show something in this
   response, render it (ui's toast).
3. When the final header is written (a 1xx is ignored; a `Flush` counts;
   or when the handler returns without writing; never after a hijack), the
   middleware decides:
   - **delivered**: a response with a `text/html` content type and a status
     below 300 or from 400 to 599, to a request that received the
     messages. Only such a page can have shown them. A `text/plain` 404 for
     `/favicon.ico`, a JSON answer or a download delivers nothing. The
     content type is the one `net/http` would send: when the handler sets
     none, the middleware sniffs the first write with
     `http.DetectContentType`, as `net/http` does, so a templ page written
     straight to `w` counts. An HTML page that renders no toast region
     still counts as delivered, and its messages are lost: the app's shell
     (ui's layouts) shows them on every page;
   - a response with status 500 or more drops what was added during this
     request (a "saved" added before the failure isn't true);
   - `Set-Cookie` is written only when something changed: the delivered
     messages are removed, or messages were added (appended to those the
     request arrived with). A response that delivers nothing and adds
     nothing leaves the cookie alone, so a slow fragment can't write back
     a stale copy.

   So `POST → 303 → GET` shows the flash on the GET; `POST → 303 → GET →
   303 (sign in) → GET` delivers it to the sign-in page, the first page; an
   htmx form answered by `Redirect` (`HX-Redirect`) shows it on the page
   htmx loads; a boosted form answered with a plain 303, which htmx
   follows, shows it on the page it swaps in.

   A cookie is one value per browser, so two tabs share it: a page that
   loads in another tab between the redirect and its target can show the
   message there. That is what a cookie flash is; a server-side store would
   need a session, which handlers doesn't have.

**The cookie (N7).** `__Host-flash` (`FlashName` changes the base),
`Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=300`; with
`FlashInsecure`, `flash` without `Secure`, as auth's `cookie.insecure`
does. Its value is base64url JSON: per message a level and either a key
with typed args or a text. It isn't signed: it holds nothing secret, every
renderer escapes it, and the `__Host-` prefix keeps a sibling subdomain
from setting it, so only the person's own browser (or script already
running on the site) can forge one, and only for themselves. A forged key
renders as that key's text with the given arguments, at worst with
`%!d(…)` where they don't fit. A malformed cookie, an unknown level or an unknown argument
tag is dropped (logged at debug), never a 500. When the messages would pass
3 KiB, the oldest are dropped (logged at warn).

The middleware wraps the `ResponseWriter` to write the cookie with the
header; the wrapper has `Unwrap`, `Flush` and `Hijack`. `Add` outside the
middleware panics rather than lose a message: a test of a handler that
flashes wraps it with `fl.Middleware`.

### 5. Later: request ids and access logs

They join `middleware` as `func(http.Handler) http.Handler` middleware.
Recovery's log line will carry the request id when there is one. Nothing
here needs to change for them.

### 6. A permission taken from another routes package (N9, N10)

Today `//h:perm <key>` declares the permission in the route's package, and a
key in two routes files stops the `permissions` generator. A route now
takes a permission another package declares by naming that package:

```go
//h:key talks.review
//h:method GET
//h:path /talks/{id}/review
//h:perm api.talks.review from apiroutes
func (h *H) TalkReview(w http.ResponseWriter, r *http.Request, id ids.TalkID) error
```

```yaml
x-permission: talks.review from adminroutes
```

- `perm`'s value is `<key>`, `<key> <text>` (§7 below) or
  `<key> from <package>`. When the word after the key is `from`, the value
  is a reference, and it must be exactly `from` and one Go identifier:
  `h:perm: write "h:perm <key> from <package>", with nothing after`
  (`x-permission: write "<key> from <package>", with nothing after`). So a
  text can't begin with the word "from", and a text added after a
  reference by mistake is an error, not a declaration.
- `<package>` is the routes package's name (its `-package`). Two routes
  packages of one module with the same name can't be taken from: the
  generator says so, and `-package` renames one.
- **Why a marker (N9).** Without one, a key in two packages can't say which
  declares it: the first generated would declare it and the second refer
  to it, depending on order, and "declaring it twice stays an error" would
  have nothing to stand on. Naming the package also says where to look.
- **The reference is checked where every routes file is seen (N10):** the
  `permissions` generator (genx gives it every `routes/v1` file of the
  run) and `vet` (the module's committed files, with the vetted packages in
  their place). The producer checks only its own file: checking the other
  package there would recreate F4's order problem (add `x-permission` and
  the admin route in one edit, and the admin producer runs before the
  spec's).
- A route taking a permission agrees with the declaration on `unverified`.
- Nothing is generated for a reference. The declaring package's `Perm`
  variable registers the permission with auth, and `authguard`'s `Prepare`
  still refuses a route whose permission isn't declared. The generated
  package never imports another routes package: two packages may take
  from each other, which an import would make a cycle. So a binary that
  mounts a taking package also links the declaring one (mounts it, or
  imports it blank); if it doesn't, `Register` panics at startup with
  `authguard: route "<key>": permission "<p>" isn't declared`, as today.
- `routes/v1`: a permission entry with `from` (a package name, not the
  file's own) has `key` and `level`, and no `var` or `text`. A file whose
  permissions are all taken writes no `zz_<package>_permissions.go`.
- The warning `h:perm "<k>" is used by this route only` (only a warning,
  so not an order problem) also counts the routes of the module's
  committed routes files that take the permission.
- In OpenAPI messages the prefix is `x-permission:` where §5.2's is
  `h:perm:`.

### 7. A permission's text (N11)

`//h:perm <key> <text>`, and `x-permission: <key> <text>`. The text is the
permission's English text (`permission.<key>` in i18n, through auth):

1. the text given, if a route of the declaring package gives one; routes
   that give one must give the same;
2. else, as today, its route's text when one route of the declaring
   package uses it;
3. else its key made readable.

A reference takes no text: the declaration's is the permission's. Rule 2
counts only the declaring package's routes, so the text doesn't depend on
which package is generated first; give a shared permission its text.

## Questions for epistemer

Each has a default; ⚠ marks a reversal of something locked or documented.

| # | Question | Default |
|---|---|---|
| N1 | ⚠ Reverses the brief's D17 (flash and htmx out of scope), as D4 decided: `handlers/middleware` in the root module, with `Recover`, `HTMX`, `Boosted`, `Redirect`, `ErrNotLocal`, `ErrorPage` and `Flash` | Yes |
| N2 | ⚠ Changes `Register` and `Mount` (a panic no longer reaches `net/http`): recovery outermost, on by default, no opt-out; a started response is aborted | Yes, no opt-out |
| N3 | `ErrPanic` (key `errors.handlers.panic`, permanent) and `Panic` live in the core with the other declared errors, because the core recovers and `middleware` imports the core. Alternative: in `middleware`, with the core reaching them through an internal package and type aliases | In the core |
| N4 | The flash's methods: `Add(ctx, level string, msg i18n.Ter, args ...any)` and `Messages(ctx) iter.Seq2[string, i18n.Ter]`, levels as four untyped string constants `Level*`, so ui's interface matches structurally | Yes |
| N5 | A key travels as key and args and renders in the reader's locale; an error or another `Ter` renders when added, in the writer's | Yes |
| N6 | Delivery: dropped by the first `text/html` page (not a redirect or a 204) to a request that isn't `HEAD` or a non-boosted htmx request; a 5xx drops what its request added; `Set-Cookie` only when something changed; `Messages` doesn't return this request's own adds | Yes |
| N7 | The cookie: `__Host-flash`, unsigned, `Max-Age` 300 s. Alternative: signed with a key from the app's settings (a key to rotate). And plain HTTP in development through an option, `FlashInsecure()`; the alternative is a getter interface, `NewFlash(c interface{ FlashCookieInsecure() bool })`, with a cfgx schema fragment, as auth reads `cookie.insecure`. An option because it is one bool fixed at startup that an app usually ties to auth's own setting (`if s.AuthCookieInsecure() { … }`), and a schema fragment would add a second setting to keep in step | Unsigned; the option |
| N8 | `Redirect` answers htmx with 204 and `HX-Redirect` (a full load, as ui's `Reply.Go`), not `HX-Location` (an htmx swap); `ErrNotLocal` is a declared 400 (`errors.handlers.not_local`), since a return address comes from the request | `HX-Redirect`; 400 |
| N9 | ⚠ Reverses §9's "a permission belongs to one routes package", as D6 decided; ⚠ also departs from your answer's bare `//h:perm api.talks.review`, which can't say which package declares a key two packages name. A reference is written `<key> from <package>`, in `perm` and `x-permission`; the word `from` after the key always makes a reference, with nothing after the package. Alternative: a directive of its own, `//h:uses <key> <package>`, with `x-permission-from` | `from <package>` |
| N10 | ⚠ Departs from your answer's "gen checks that the key exists": the reference is checked by the `permissions` generator (which runs within `go tool genx`) and `vet`, not by the producer `handlers gen`, which can't see the other package's new file when both change in one edit | Yes |
| N11 | Text after the key; sharers must agree; a reference has none; without a text, today's rule counting the declaring package's routes only | Yes |

## Consequences

- Confab deletes `Recover` (recovery is on by default), `flash.go` (the
  middleware's flash, shown through ui once ui mirrors it), and the second
  grant of every shared permission (`api.talks.review` is taken by the
  admin route). F4's stub is amendment A6, already built.
- Minor release `v0.4.0` of all four modules: the root (middleware,
  recovery, `ErrPanic`, `Panic`), `cmd` (the grammar, both producers,
  `vet`, the language server), and `authguard` and `openapi` requiring the
  new root.
- `routes/v1` gains an optional field; files written before stay valid.
- A permission that a package stops declaring and takes from another
  instead stays in its routes file (with `from`), so the removed-permission
  warning doesn't name it; its grants stay valid while the other package
  declares it.

// Package middleware owns the HTTP request lifecycle for apps built on
// handlers: panic recovery for an app's own handlers (Recover), htmx request
// inspection (HTMX, Boosted), htmx-aware local redirects (Redirect,
// ErrNotLocal), HTML error-page rendering (ErrorPage), and cross-redirect
// flash messages (Flash).
//
// Request IDs and access logs will join this package later.
package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/internal/recovery"
)

// ErrNotLocal is returned by Redirect when given an address that is not on
// this site, such as an untrusted return URL from a query parameter.
var ErrNotLocal = errors.Key("errors.handlers.not_local", "the address isn't this site's", errors.Invalid)

// Recover recovers a panic in next as route registration does, logging method
// and path and writing ErrPanic with ew. Routes registered by Register and
// Mount already recover.
func Recover(ew handlers.ErrorWriter) func(http.Handler) http.Handler {
	if ew == nil {
		ew = handlers.WriteText
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recovery.Serve(w, r, "", ew, wrapPanic, handlers.WriteText, next.ServeHTTP)
		})
	}
}

func wrapPanic(val any, stack []byte) error {
	return handlers.ErrPanic.Wrap(&handlers.Panic{Value: val, Stack: stack})
}

// HTMX reports whether htmx sent r (HX-Request: true).
func HTMX(r *http.Request) bool {
	return r != nil && r.Header.Get("HX-Request") == "true"
}

// Boosted reports whether a boosted link or form sent r (HX-Boosted: true).
func Boosted(r *http.Request) bool {
	return r != nil && r.Header.Get("HX-Boosted") == "true"
}

// Redirect sends the browser to to: with htmx, 204 No Content and HX-Redirect
// so htmx loads it as a whole page; without htmx, 303 See Other. An address
// that isn't this site's (not one leading slash, a scheme or a host, a
// backslash, whitespace, or a control character) returns ErrNotLocal and writes
// nothing. A nil to panics.
func Redirect(w http.ResponseWriter, r *http.Request, to interface{ URL() string }) error {
	if to == nil {
		panic("middleware: Redirect with nil target")
	}
	u, ok := isLocal(to.URL())
	if !ok {
		return ErrNotLocal
	}
	if HTMX(r) {
		w.Header().Set("HX-Redirect", u)
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	http.Redirect(w, r, u, http.StatusSeeOther)
	return nil
}

func isLocal(u string) (string, bool) {
	if !strings.HasPrefix(u, "/") || strings.HasPrefix(u, "//") || strings.ContainsRune(u, '\\') {
		return u, false
	}
	for _, c := range u {
		if c <= ' ' || c == 0x7f || unicode.IsControl(c) || unicode.IsSpace(c) {
			return u, false
		}
	}
	p, err := url.Parse(u)
	return u, err == nil && p.Scheme == "" && p.Host == "" && !strings.HasPrefix(p.Path, "//")
}

// ErrorPage returns an ErrorWriter for handlers.WithErrors. It logs err as
// handlers.WriteText does (status 500 or more, ErrPanic excepted), then calls
// page with handlers.Status(err); page writes the whole answer, status
// included. If page returns an error, that is logged too; if nothing was
// written, err is written as WriteText writes it without logging it again; if
// something was written, the connection is aborted.
func ErrorPage(page func(w http.ResponseWriter, r *http.Request, status int, err error) error) handlers.ErrorWriter {
	if page == nil {
		panic("middleware: nil ErrorPage")
	}
	return func(w http.ResponseWriter, r *http.Request, err error) {
		status := handlers.Status(err)
		if status >= http.StatusInternalServerError && !errors.Is(err, handlers.ErrPanic) {
			msg := "nil error"
			if err != nil {
				msg = err.Error()
			}
			slog.Default().ErrorContext(r.Context(), msg, "status", status)
		}

		rw := recovery.NewWriter(w)
		pageErr := page(rw, r, status, err)
		if pageErr == nil {
			return
		}

		slog.Default().ErrorContext(r.Context(), "middleware: error page", "err", pageErr)
		if rw.Started() {
			panic(http.ErrAbortHandler)
		}

		clear(rw.Header())
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		if err != nil {
			_, _ = io.WriteString(w, errors.T(r.Context(), err))
		}
	}
}

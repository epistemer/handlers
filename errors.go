package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/epistemer/errors"
	"github.com/epistemer/i18n"
)

// Guard admits requests to non-public routes.
type Guard interface {
	// Prepare is called once per route that isn't public, at registration;
	// an error makes registration panic.
	Prepare(rt Route) error
	// Check admits a request to a route that isn't public, or returns the
	// error to answer with.
	Check(r *http.Request, rt Route) error
	// Admit returns a check for many routes, with the request's session
	// read once. A request without a session, or without a permission, gets
	// a check that refuses those routes; the error is for a failure to read
	// the session or the permissions.
	Admit(r *http.Request) (func(Route) bool, error)
}

type options struct {
	errWriter ErrorWriter
	maxBody   int64
	mw        []func(http.Handler) http.Handler
}

func defaultOptions() options {
	return options{
		maxBody: 1 << 20,
	}
}

// Option configures route registration on Mount.
type Option func(*options)

// WithErrors sets the ErrorWriter used when a guard or handler returns an
// error, or when a prefixed catch-all answers not found. The default is
// WriteText for HTML routes and WriteJSON for JSON routes.
func WithErrors(w ErrorWriter) Option {
	return func(o *options) {
		o.errWriter = w
	}
}

// WithMaxBody sets the maximum request body size in bytes, defaulting to
// 1 MiB. Zero disables the limit; a negative value panics.
func WithMaxBody(n int64) Option {
	if n < 0 {
		panic("handlers: negative max body")
	}
	return func(o *options) {
		o.maxBody = n
	}
}

// WithMiddleware appends HTTP middleware to run in the order given, after
// the guard and before parameter binding.
func WithMiddleware(mw ...func(http.Handler) http.Handler) Option {
	for _, m := range mw {
		if m == nil {
			panic("handlers: nil middleware")
		}
	}
	return func(o *options) {
		o.mw = append(o.mw, mw...)
	}
}

// ErrorWriter writes an HTTP error response for err.
type ErrorWriter func(w http.ResponseWriter, r *http.Request, err error)

// Public sentinel errors returned by handlers' binding, body limit, and routing.
var (
	ErrBadParam       = errors.Key1[i18n.TKer]("errors.handlers.bad_param", "%s is not valid", errors.Invalid)
	ErrNotFound       = errors.Key("errors.handlers.not_found", "not found", errors.NotFound)
	ErrTooLarge       = errors.Key("errors.handlers.too_large", "the request is too large", errors.Invalid)
	ErrInvalidRequest = errors.Key("errors.handlers.invalid_request", "the request is not valid", errors.Invalid)
	ErrInvalidField   = errors.Key1[string]("errors.handlers.invalid_field", "the request is not valid at %s", errors.Invalid)
)

// Status returns the HTTP status code for err: 413 for ErrTooLarge or
// *http.MaxBytesError, otherwise errors.KindOf(err).Status().
func Status(err error) int {
	if errors.Is(err, ErrTooLarge) {
		return http.StatusRequestEntityTooLarge
	}
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return http.StatusRequestEntityTooLarge
	}
	return errors.KindOf(err).Status()
}

// WriteText writes err as text/plain; charset=utf-8 with errors.T in the
// request's locale, logging errors with status 500 or greater via slog.Default.
func WriteText(w http.ResponseWriter, r *http.Request, err error) {
	status := Status(err)
	if status >= http.StatusInternalServerError {
		logServerError(status, err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	if err != nil {
		_, _ = io.WriteString(w, errors.T(r.Context(), err))
	}
}

// WriteJSON writes err as application/json with {"code": ..., "message": ...},
// logging errors with status 500 or greater via slog.Default.
func WriteJSON(w http.ResponseWriter, r *http.Request, err error) {
	status := Status(err)
	if status >= http.StatusInternalServerError {
		logServerError(status, err)
	}
	var code, msg string
	if pub := errors.Public(err); pub != nil {
		code = pub.K()
		msg = errors.T(r.Context(), err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{
		Code:    code,
		Message: msg,
	})
}

func logServerError(status int, err error) {
	msg := "nil error"
	if err != nil {
		msg = err.Error()
	}
	slog.Default().Error(msg, "status", status)
}

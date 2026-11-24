// Package recovery provides shared HTTP panic recovery and response-start
// tracking for handlers and handlers/middleware.
package recovery

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
)

// Writer wraps an http.ResponseWriter to track whether a final response has
// started (a non-1xx WriteHeader, Write, Flush, or successful Hijack).
type Writer struct {
	w       http.ResponseWriter
	started bool
}

// NewWriter returns a Writer wrapping w.
func NewWriter(w http.ResponseWriter) *Writer {
	return &Writer{w: w}
}

// Started reports whether the final response has started.
func (w *Writer) Started() bool {
	return w.started
}

// Header returns the underlying header map.
func (w *Writer) Header() http.Header {
	return w.w.Header()
}

// WriteHeader marks the response as started for non-1xx status codes and
// delegates to the underlying writer.
func (w *Writer) WriteHeader(code int) {
	if code < 100 || code > 199 {
		w.started = true
	}
	w.w.WriteHeader(code)
}

// Write marks the response as started and writes p to the underlying writer.
func (w *Writer) Write(p []byte) (int, error) {
	w.started = true
	return w.w.Write(p)
}

// Unwrap returns the underlying http.ResponseWriter for http.ResponseController.
func (w *Writer) Unwrap() http.ResponseWriter {
	return w.w
}

// Flush marks the response as started and flushes the underlying writer if supported.
func (w *Writer) Flush() {
	w.started = true
	_ = http.NewResponseController(w.w).Flush()
}

// FlushError flushes the underlying writer via http.ResponseController and marks
// the response as started on success.
func (w *Writer) FlushError() error {
	err := http.NewResponseController(w.w).Flush()
	if err == nil {
		w.started = true
	}
	return err
}

// Hijack delegates to an underlying http.Hijacker (following Unwrap), or returns
// http.ErrNotSupported when the underlying writer does not support hijacking.
func (w *Writer) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	rw := w.w
	for {
		if h, ok := rw.(http.Hijacker); ok {
			conn, brw, err := h.Hijack()
			if err == nil {
				w.started = true
			}
			return conn, brw, err
		}
		u, ok := rw.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			return nil, nil, http.ErrNotSupported
		}
		rw = u.Unwrap()
	}
}

// Serve runs next with panic recovery. If route is non-empty, panics are logged
// with the "route" attribute; otherwise they are logged with "method" and "path".
func Serve(
	w http.ResponseWriter,
	r *http.Request,
	route string,
	ew func(http.ResponseWriter, *http.Request, error),
	wrapPanic func(any, []byte) error,
	fallback func(http.ResponseWriter, *http.Request, error),
	next func(http.ResponseWriter, *http.Request),
) {
	rw := NewWriter(w)
	var panicErr error
	// The headers as they were before next ran: outer middleware's (a
	// Content-Security-Policy, a flash) stay on the error page; what next set
	// (a Content-Disposition, a Content-Encoding) doesn't.
	before := w.Header().Clone()
	restore := func() {
		h := rw.Header()
		clear(h)
		for k, v := range before {
			h[k] = v
		}
	}

	defer func() {
		if panicErr == nil {
			return
		}
		// If ew panicked, recover here.
		if v2 := recover(); v2 != nil {
			if v2 == http.ErrAbortHandler {
				panic(http.ErrAbortHandler)
			}
			logPanic(r, route, v2, debug.Stack())
			if rw.started {
				panic(http.ErrAbortHandler)
			}
			restore()
			defer func() {
				if recover() != nil {
					panic(http.ErrAbortHandler)
				}
			}()
			fallback(rw, r, panicErr)
		}
	}()

	func() {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(http.ErrAbortHandler)
			}
			stack := debug.Stack()
			logPanic(r, route, v, stack)
			if rw.started {
				panic(http.ErrAbortHandler)
			}
			panicErr = wrapPanic(v, stack)
		}()
		next(rw, r)
	}()

	if panicErr != nil {
		restore()
		if ew != nil {
			ew(rw, r, panicErr)
		} else {
			fallback(rw, r, panicErr)
		}
	}
}

func logPanic(r *http.Request, route string, val any, stack []byte) {
	safeVal := safePanicAttr(val)
	ctx := r.Context()
	if route != "" {
		slog.Default().ErrorContext(ctx, "handlers: panic",
			slog.String("route", route),
			slog.Any("panic", safeVal),
			slog.String("stack", string(stack)),
		)
		return
	}
	var path string
	if r.URL != nil {
		path = r.URL.Path
	}
	slog.Default().ErrorContext(ctx, "handlers: panic",
		slog.String("method", r.Method),
		slog.String("path", path),
		slog.Any("panic", safeVal),
		slog.String("stack", string(stack)),
	)
}

func safePanicAttr(v any) any {
	var panics bool
	func() {
		defer func() {
			if recover() != nil {
				panics = true
			}
		}()
		switch x := v.(type) {
		case error:
			_ = x.Error()
		case fmt.Stringer:
			_ = x.String()
		default:
			_ = fmt.Sprint(v)
		}
	}()
	if panics {
		return fmt.Sprintf("%T", v)
	}
	return v
}

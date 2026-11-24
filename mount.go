package handlers

import (
	"encoding"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/epistemer/handlers/internal/recovery"
)

// Mount registers a route table on an http.ServeMux. Generated Register and
// Mount functions call NewMount, Handle, and Done.
type Mount struct {
	mux     *http.ServeMux
	routes  []Route
	guard   Guard
	opts    options
	handled []bool
}

// NewMount creates a Mount for registering routes on mux with guard g and opts.
// It panics if mux is nil.
func NewMount(mux *http.ServeMux, routes []Route, g Guard, opts ...Option) *Mount {
	if mux == nil {
		panic("handlers: nil mux")
	}
	o := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return &Mount{
		mux:     mux,
		routes:  routes,
		guard:   g,
		opts:    o,
		handled: make([]bool, len(routes)),
	}
}

// Handle registers routes[i] on the mux with the request chain: body limit,
// route in context, guard check (unless public), middleware, and then h.
func (m *Mount) Handle(i int, h func(w http.ResponseWriter, r *http.Request) error) {
	m.handle(i, h == nil, func(ew ErrorWriter) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := h(w, r); err != nil {
				ew(w, r, err)
			}
		})
	})
}

// HandleHTTP registers routes[i] like Handle, with h as the handler and
// validate (if not nil) run after the middleware: the API's step 5. h writes
// its own response.
func (m *Mount) HandleHTTP(i int, h http.Handler, validate func(*http.Request) error) {
	m.handle(i, h == nil, func(ew ErrorWriter) http.Handler {
		if validate == nil {
			return h
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := validate(r); err != nil {
				ew(w, r, err)
				return
			}
			h.ServeHTTP(w, r)
		})
	})
}

// Writer returns the error writer the options chose, or the default writer
// for the mounted routes' surface (nil when routes is empty and WithErrors
// was not given).
func (m *Mount) Writer() ErrorWriter {
	if m.opts.errWriter != nil {
		return m.opts.errWriter
	}
	if len(m.routes) > 0 {
		return m.errorWriterFor(m.routes[0].Surface)
	}
	return nil
}

func (m *Mount) handle(i int, handlerNil bool, build func(ew ErrorWriter) http.Handler) {
	if i < 0 || i >= len(m.routes) {
		panic(fmt.Sprintf("handlers: route index %d out of range", i))
	}
	if m.handled[i] {
		panic(fmt.Sprintf("handlers: route %s (%d) already handled", m.routes[i].Key, i))
	}
	if handlerNil {
		panic(fmt.Sprintf("handlers: nil handler for route %s", m.routes[i].Key))
	}
	m.handled[i] = true

	rt := m.routes[i]
	ew := m.errorWriterFor(rt.Surface)

	switch rt.Auth {
	case Public:
	case Session, Permission:
		if m.guard == nil {
			panic(fmt.Sprintf("handlers: nil guard for route %s", rt.Key))
		}
		if err := m.guard.Prepare(rt); err != nil {
			panic(fmt.Sprintf("handlers: guard prepare %s: %v", rt.Key, err))
		}
	default:
		panic(fmt.Sprintf("handlers: unknown auth %d", rt.Auth))
	}

	chained := build(ew)
	for idx := len(m.opts.mw) - 1; idx >= 0; idx-- {
		chained = m.opts.mw[idx](chained)
	}

	pat := rt.Pattern
	if pat == "/" {
		pat = "/{$}"
	}
	m.mux.HandleFunc(rt.Method+" "+rt.Prefix+pat, func(w http.ResponseWriter, r *http.Request) {
		recovery.Serve(w, r, rt.Key, ew, wrapPanic, WriteText, func(w http.ResponseWriter, r *http.Request) {
			limit := m.opts.maxBody
			if rt.MaxBody > 0 {
				limit = rt.MaxBody
			}
			if limit > 0 && r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			r = r.WithContext(WithRoute(r.Context(), rt))
			if rt.Auth != Public {
				if err := m.guard.Check(r, rt); err != nil {
					ew(w, r, err)
					return
				}
			}
			chained.ServeHTTP(w, r)
		})
	})
}

// Done verifies that every route was handled and, under a non-empty prefix,
// registers the catch-all <prefix>/ handler answering ErrNotFound.
func (m *Mount) Done() {
	for i, ok := range m.handled {
		if !ok {
			panic(fmt.Sprintf("handlers: route %s (%d) was not handled", m.routes[i].Key, i))
		}
	}
	if len(m.routes) == 0 {
		return
	}
	prefix := m.routes[0].Prefix
	if prefix == "" {
		return
	}
	ew := m.errorWriterFor(m.routes[0].Surface)
	m.mux.HandleFunc(prefix+"/", func(w http.ResponseWriter, r *http.Request) {
		recovery.Serve(w, r, "", ew, wrapPanic, WriteText, func(w http.ResponseWriter, r *http.Request) {
			ew(w, r, ErrNotFound)
		})
	})
}

func (m *Mount) errorWriterFor(s Surface) ErrorWriter {
	var def ErrorWriter
	switch s {
	case HTML:
		def = WriteText
	case JSON:
		def = WriteJSON
	default:
		panic(fmt.Sprintf("handlers: unknown surface %d", s))
	}
	if m.opts.errWriter != nil {
		return m.opts.errWriter
	}
	return def
}

// ParamString returns the path parameter p from r.
func ParamString(r *http.Request, p Param) string {
	return r.PathValue(p.Name)
}

// ParamInt parses path parameter p as a signed integer of bit width bits
// (where 0 uses strconv.IntSize). On failure it returns ErrBadParam.With(p.Label).
func ParamInt(r *http.Request, p Param, bits int) (int64, error) {
	v, err := strconv.ParseInt(r.PathValue(p.Name), 10, bits)
	if err != nil {
		return 0, ErrBadParam.With(p.Label)
	}
	return v, nil
}

// ParamUint parses path parameter p as an unsigned integer of bit width bits
// (where 0 uses strconv.IntSize). On failure it returns ErrBadParam.With(p.Label).
func ParamUint(r *http.Request, p Param, bits int) (uint64, error) {
	v, err := strconv.ParseUint(r.PathValue(p.Name), 10, bits)
	if err != nil {
		return 0, ErrBadParam.With(p.Label)
	}
	return v, nil
}

// ParamBool parses path parameter p as a boolean. On failure it returns
// ErrBadParam.With(p.Label).
func ParamBool(r *http.Request, p Param) (bool, error) {
	v, err := strconv.ParseBool(r.PathValue(p.Name))
	if err != nil {
		return false, ErrBadParam.With(p.Label)
	}
	return v, nil
}

// ParamText unmarshals path parameter p into v via encoding.TextUnmarshaler.
// On failure it returns ErrBadParam.With(p.Label).
func ParamText(r *http.Request, p Param, v encoding.TextUnmarshaler) error {
	if err := v.UnmarshalText([]byte(r.PathValue(p.Name))); err != nil {
		return ErrBadParam.With(p.Label)
	}
	return nil
}

// Segment escapes s as a single URL path segment.
func Segment(s string) string {
	return url.PathEscape(s)
}

// Rest escapes each slash-separated segment of s while preserving slashes.
func Rest(s string) string {
	parts := strings.Split(s, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// Text encodes v via MarshalText and panics if MarshalText returns an error.
func Text(v encoding.TextMarshaler) string {
	b, err := v.MarshalText()
	if err != nil {
		panic(fmt.Sprintf("handlers: MarshalText: %v", err))
	}
	return string(b)
}

package handlers_test

import (
	"bufio"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
)

type badStringer struct{}

func (badStringer) String() string {
	panic("boom in String")
}

type badError struct{}

func (badError) Error() string {
	panic("boom in Error")
}

type hijackSpy struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackSpy) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func TestPanicError(t *testing.T) {
	p1 := &handlers.Panic{Value: "something broke"}
	if got := p1.Error(); got != "panic: something broke" {
		t.Errorf("Panic.Error() = %q, want %q", got, "panic: something broke")
	}

	p2 := &handlers.Panic{Value: badStringer{}}
	if got := p2.Error(); got != "panic: handlers_test.badStringer" {
		t.Errorf("Panic.Error() with panicking String = %q, want %q", got, "panic: handlers_test.badStringer")
	}

	p3 := &handlers.Panic{Value: badError{}}
	if got := p3.Error(); got != "panic: handlers_test.badError" {
		t.Errorf("Panic.Error() with panicking Error = %q, want %q", got, "panic: handlers_test.badError")
	}

	var pNil *handlers.Panic
	if got := pNil.Error(); got != "panic: <nil>" {
		t.Errorf("(*Panic)(nil).Error() = %q, want %q", got, "panic: <nil>")
	}
}

func TestDefaultWritersDoNotDoubleLogErrPanic(t *testing.T) {
	orig := slog.Default()
	cap := &logCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(orig) })

	err := handlers.ErrPanic.Wrap(&handlers.Panic{Value: "boom"})

	recText := httptest.NewRecorder()
	handlers.WriteText(recText, httptest.NewRequest("GET", "/", nil), err)
	if recText.Code != 500 || recText.Body.String() != "the server failed to answer" {
		t.Errorf("WriteText(ErrPanic) = (%d, %q)", recText.Code, recText.Body.String())
	}

	recJSON := httptest.NewRecorder()
	handlers.WriteJSON(recJSON, httptest.NewRequest("GET", "/", nil), err)
	if recJSON.Code != 500 || !strings.Contains(recJSON.Body.String(), `"errors.handlers.panic"`) {
		t.Errorf("WriteJSON(ErrPanic) = (%d, %q)", recJSON.Code, recJSON.Body.String())
	}

	if len(cap.records) != 0 {
		t.Errorf("expected no slog records for ErrPanic in default writers, got %v", cap.records)
	}
}

func TestMountRecovery(t *testing.T) {
	captureLogs := func(t *testing.T) *logCapture {
		t.Helper()
		orig := slog.Default()
		cap := &logCapture{}
		slog.SetDefault(slog.New(cap))
		t.Cleanup(func() { slog.SetDefault(orig) })
		return cap
	}

	t.Run("panicking handler passes ErrPanic with *Panic and logs once with route", func(t *testing.T) {
		cap := captureLogs(t)
		routes := []handlers.Route{
			{Key: "orders.boom", Method: "GET", Prefix: "/app", Pattern: "/boom", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		var gotErr error
		var gotPanic *handlers.Panic
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithErrors(func(w http.ResponseWriter, r *http.Request, err error) {
			gotErr = err
			errors.As(err, &gotPanic)
			handlers.WriteText(w, r, err)
		}))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			panic("kaboom")
		})
		m.Done()

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/app/boom", nil))

		if rec.Code != 500 || rec.Body.String() != "the server failed to answer" {
			t.Errorf("response = (%d, %q)", rec.Code, rec.Body.String())
		}
		if !errors.Is(gotErr, handlers.ErrPanic) {
			t.Errorf("error writer got %v, want ErrPanic", gotErr)
		}
		if gotPanic == nil || gotPanic.Value != "kaboom" || len(gotPanic.Stack) == 0 {
			t.Errorf("gotPanic = %+v, want Value=kaboom and non-empty Stack", gotPanic)
		}
		if len(cap.records) != 1 {
			t.Fatalf("expected 1 log record, got %d: %v", len(cap.records), cap.records)
		}
		log := cap.records[0]
		if !strings.HasPrefix(log, "ERROR handlers: panic ") ||
			!strings.Contains(log, "route=orders.boom") ||
			!strings.Contains(log, "panic=kaboom") ||
			!strings.Contains(log, "stack=") {
			t.Errorf("unexpected log record: %q", log)
		}
	})

	t.Run("panic in guard Check and WithMiddleware is recovered", func(t *testing.T) {
		cap := captureLogs(t)
		routes := []handlers.Route{
			{Key: "guard.panic", Method: "GET", Prefix: "/app", Pattern: "/g", Surface: handlers.HTML, Auth: handlers.Session, Label: testRouteLabel},
			{Key: "mw.panic", Method: "GET", Prefix: "/app", Pattern: "/m", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		g := &panicGuard{}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, g, handlers.WithMiddleware(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				panic("mw boom")
			})
		}))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error { return nil })
		m.Handle(1, func(w http.ResponseWriter, r *http.Request) error { return nil })
		m.Done()

		recG := httptest.NewRecorder()
		mux.ServeHTTP(recG, httptest.NewRequest("GET", "/app/g", nil))
		if recG.Code != 500 || recG.Body.String() != "the server failed to answer" {
			t.Errorf("guard panic response = (%d, %q)", recG.Code, recG.Body.String())
		}

		recM := httptest.NewRecorder()
		mux.ServeHTTP(recM, httptest.NewRequest("GET", "/app/m", nil))
		if recM.Code != 500 || recM.Body.String() != "the server failed to answer" {
			t.Errorf("mw panic response = (%d, %q)", recM.Code, recM.Body.String())
		}

		if len(cap.records) != 2 {
			t.Fatalf("expected 2 log records, got %d: %v", len(cap.records), cap.records)
		}
		if !strings.Contains(cap.records[0], "route=guard.panic") || !strings.Contains(cap.records[0], "panic=guard boom") {
			t.Errorf("guard log = %q", cap.records[0])
		}
		if !strings.Contains(cap.records[1], "route=mw.panic") || !strings.Contains(cap.records[1], "panic=mw boom") {
			t.Errorf("mw log = %q", cap.records[1])
		}
	})

	t.Run("catch-all in Done is recovered", func(t *testing.T) {
		cap := captureLogs(t)
		routes := []handlers.Route{
			{Key: "home", Method: "GET", Prefix: "/app", Pattern: "/", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithErrors(func(w http.ResponseWriter, r *http.Request, err error) {
			panic("catch-all writer panic")
		}))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error { return nil })
		m.Done()

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/app/missing", nil))
		if rec.Code != 500 || rec.Body.String() != "the server failed to answer" {
			t.Errorf("catch-all panic fallback = (%d, %q)", rec.Code, rec.Body.String())
		}
		if len(cap.records) != 2 {
			t.Fatalf("expected 2 log records (catch-all panic + ew panic), got %d: %v", len(cap.records), cap.records)
		}
		if !strings.Contains(cap.records[0], "method=GET") || !strings.Contains(cap.records[0], "path=/app/missing") {
			t.Errorf("catch-all log = %q", cap.records[0])
		}
	})

	t.Run("http.ErrAbortHandler is re-panicked as is without logging", func(t *testing.T) {
		cap := captureLogs(t)
		routes := []handlers.Route{
			{Key: "abort", Method: "GET", Prefix: "/app", Pattern: "/abort", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil)
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			panic(http.ErrAbortHandler)
		})
		m.Done()

		var recovered any
		func() {
			defer func() { recovered = recover() }()
			mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/app/abort", nil))
		}()
		if recovered != http.ErrAbortHandler {
			t.Errorf("recovered = %v, want http.ErrAbortHandler", recovered)
		}
		if len(cap.records) != 0 {
			t.Errorf("expected no logs for ErrAbortHandler, got %v", cap.records)
		}
	})

	t.Run("panic after Write aborts with http.ErrAbortHandler", func(t *testing.T) {
		cap := captureLogs(t)
		routes := []handlers.Route{
			{Key: "partial", Method: "GET", Prefix: "/app", Pattern: "/partial", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		ewCalled := false
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithErrors(func(w http.ResponseWriter, r *http.Request, err error) {
			ewCalled = true
		}))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			_, _ = io.WriteString(w, "partial")
			panic("after write")
		})
		m.Done()

		var recovered any
		rec := httptest.NewRecorder()
		func() {
			defer func() { recovered = recover() }()
			mux.ServeHTTP(rec, httptest.NewRequest("GET", "/app/partial", nil))
		}()
		if recovered != http.ErrAbortHandler {
			t.Errorf("recovered = %v, want http.ErrAbortHandler", recovered)
		}
		if ewCalled {
			t.Error("error writer should not be called after response started")
		}
		if len(cap.records) != 1 || !strings.Contains(cap.records[0], "panic=after write") {
			t.Errorf("logs = %v", cap.records)
		}
	})

	t.Run("panicking error writer falls back to WriteText with headers cleared", func(t *testing.T) {
		cap := captureLogs(t)
		routes := []handlers.Route{
			{Key: "ew.panic", Method: "GET", Prefix: "/app", Pattern: "/ew", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithErrors(func(w http.ResponseWriter, r *http.Request, err error) {
			w.Header().Set("X-Stale", "bad")
			w.Header().Set("Content-Type", "text/html")
			panic("render panic")
		}))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			panic("handler panic")
		})
		m.Done()

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/app/ew", nil))

		if rec.Code != 500 {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		if rec.Header().Get("X-Stale") != "" {
			t.Errorf("X-Stale header was not cleared: %q", rec.Header().Get("X-Stale"))
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
		}
		if rec.Body.String() != "the server failed to answer" {
			t.Errorf("body = %q, want %q", rec.Body.String(), "the server failed to answer")
		}
		if len(cap.records) != 2 {
			t.Fatalf("expected 2 log records (handler + ew), got %d: %v", len(cap.records), cap.records)
		}
		if !strings.Contains(cap.records[0], "panic=handler panic") || !strings.Contains(cap.records[1], "panic=render panic") {
			t.Errorf("logs = %v", cap.records)
		}
	})

	t.Run("Flush, ResponseController.Flush, and Hijack work through wrapper", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "flush", Method: "GET", Prefix: "/app", Pattern: "/flush", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
			{Key: "rcflush", Method: "GET", Prefix: "/app", Pattern: "/rcflush", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
			{Key: "hijack", Method: "GET", Prefix: "/app", Pattern: "/hijack", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil)
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			fl, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("w does not implement http.Flusher")
			}
			fl.Flush()
			panic("after flush")
		})
		m.Handle(1, func(w http.ResponseWriter, r *http.Request) error {
			if err := http.NewResponseController(w).Flush(); err != nil {
				return err
			}
			panic("after rc flush")
		})
		m.Handle(2, func(w http.ResponseWriter, r *http.Request) error {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("w does not implement http.Hijacker")
			}
			if _, _, err := hj.Hijack(); err != nil {
				return err
			}
			panic("after hijack")
		})
		m.Done()

		_ = captureLogs(t)

		// Flush -> started -> abort.
		rec1 := httptest.NewRecorder()
		var p1 any
		func() {
			defer func() { p1 = recover() }()
			mux.ServeHTTP(rec1, httptest.NewRequest("GET", "/app/flush", nil))
		}()
		if !rec1.Flushed || p1 != http.ErrAbortHandler {
			t.Errorf("Flush: flushed=%v, recovered=%v", rec1.Flushed, p1)
		}

		// ResponseController.Flush -> started -> abort.
		rec2 := httptest.NewRecorder()
		var p2 any
		func() {
			defer func() { p2 = recover() }()
			mux.ServeHTTP(rec2, httptest.NewRequest("GET", "/app/rcflush", nil))
		}()
		if !rec2.Flushed || p2 != http.ErrAbortHandler {
			t.Errorf("ResponseController.Flush: flushed=%v, recovered=%v", rec2.Flushed, p2)
		}

		// Hijack on ResponseRecorder returns http.ErrNotSupported.
		recNoHj := httptest.NewRecorder()
		mux.ServeHTTP(recNoHj, httptest.NewRequest("GET", "/app/hijack", nil))
		if recNoHj.Code != 500 {
			t.Errorf("Hijack unsupported status = %d, want 500", recNoHj.Code)
		}

		// Hijack on a Hijacker marks started -> abort on subsequent panic.
		spy := &hijackSpy{ResponseRecorder: httptest.NewRecorder()}
		var p3 any
		func() {
			defer func() { p3 = recover() }()
			mux.ServeHTTP(spy, httptest.NewRequest("GET", "/app/hijack", nil))
		}()
		if !spy.hijacked || p3 != http.ErrAbortHandler {
			t.Errorf("Hijack spy: hijacked=%v, recovered=%v", spy.hijacked, p3)
		}
	})
}

type panicGuard struct{}

func (panicGuard) Prepare(handlers.Route) error              { return nil }
func (panicGuard) Check(*http.Request, handlers.Route) error { panic("guard boom") }
func (panicGuard) Admit(*http.Request) (func(handlers.Route) bool, error) {
	return func(handlers.Route) bool { return true }, nil
}

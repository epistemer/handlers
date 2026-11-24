package middleware_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/middleware"
)

var (
	errPageTest404 = errors.Key("test.mw.not_found", "item missing", errors.NotFound)
)

type logCapture struct {
	mu      sync.Mutex
	records []string
}

func (l *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value))
		return true
	})
	l.records = append(l.records, fmt.Sprintf("%s %s %s", r.Level, r.Message, strings.Join(attrs, " ")))
	return nil
}
func (l *logCapture) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logCapture) WithGroup(string) slog.Handler      { return l }

type stringLink string

func (s stringLink) URL() string { return string(s) }

func TestRecover(t *testing.T) {
	orig := slog.Default()
	cap := &logCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(orig) })

	var gotErr error
	var gotPanic *handlers.Panic
	h := middleware.Recover(func(w http.ResponseWriter, r *http.Request, err error) {
		gotErr = err
		errors.As(err, &gotPanic)
		handlers.WriteText(w, r, err)
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("outside route")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/own/path", nil))

	if rec.Code != 500 || rec.Body.String() != "the server failed to answer" {
		t.Errorf("response = (%d, %q)", rec.Code, rec.Body.String())
	}
	if !errors.Is(gotErr, handlers.ErrPanic) || gotPanic == nil || gotPanic.Value != "outside route" {
		t.Errorf("gotErr=%v, gotPanic=%+v", gotErr, gotPanic)
	}
	if len(cap.records) != 1 {
		t.Fatalf("expected 1 log record, got %d: %v", len(cap.records), cap.records)
	}
	log := cap.records[0]
	if !strings.HasPrefix(log, "ERROR handlers: panic ") ||
		!strings.Contains(log, "method=POST") ||
		!strings.Contains(log, "path=/own/path") ||
		!strings.Contains(log, "panic=outside route") ||
		!strings.Contains(log, "stack=") {
		t.Errorf("unexpected log record: %q", log)
	}
}

func TestHTMXAndBoosted(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if middleware.HTMX(req) || middleware.Boosted(req) {
		t.Error("plain request should not be HTMX or Boosted")
	}

	reqHX := httptest.NewRequest("GET", "/", nil)
	reqHX.Header.Set("HX-Request", "true")
	if !middleware.HTMX(reqHX) || middleware.Boosted(reqHX) {
		t.Errorf("HX-Request: true -> HTMX=%v, Boosted=%v", middleware.HTMX(reqHX), middleware.Boosted(reqHX))
	}

	reqBoost := httptest.NewRequest("GET", "/", nil)
	reqBoost.Header.Set("HX-Request", "true")
	reqBoost.Header.Set("HX-Boosted", "true")
	if !middleware.HTMX(reqBoost) || !middleware.Boosted(reqBoost) {
		t.Errorf("boosted -> HTMX=%v, Boosted=%v", middleware.HTMX(reqBoost), middleware.Boosted(reqBoost))
	}
}

func TestRedirect(t *testing.T) {
	t.Run("non-htmx writes 303 and Location", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/submit", nil)
		if err := middleware.Redirect(rec, req, stringLink("/orders/42?tab=items")); err != nil {
			t.Fatalf("Redirect error = %v", err)
		}
		if rec.Code != http.StatusSeeOther {
			t.Errorf("status = %d, want 303", rec.Code)
		}
		if loc := rec.Header().Get("Location"); loc != "/orders/42?tab=items" {
			t.Errorf("Location = %q, want /orders/42?tab=items", loc)
		}
	})

	t.Run("htmx writes 204 and HX-Redirect", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/submit", nil)
		req.Header.Set("HX-Request", "true")
		if err := middleware.Redirect(rec, req, stringLink("/orders/42")); err != nil {
			t.Fatalf("Redirect error = %v", err)
		}
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204", rec.Code)
		}
		if hx := rec.Header().Get("HX-Redirect"); hx != "/orders/42" {
			t.Errorf("HX-Redirect = %q, want /orders/42", hx)
		}
		if rec.Header().Get("Location") != "" {
			t.Errorf("unexpected Location header: %q", rec.Header().Get("Location"))
		}
	})

	t.Run("non-local targets return ErrNotLocal and write nothing", func(t *testing.T) {
		bad := []string{
			"//evil",
			"https://x",
			`/a\b`,
			"/a b",
			"",
			"/a\nb",
			"/a\x7fb",
		}
		for _, target := range bad {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/submit", nil)
			err := middleware.Redirect(rec, req, stringLink(target))
			if !errors.Is(err, middleware.ErrNotLocal) {
				t.Errorf("Redirect(%q) err = %v, want ErrNotLocal", target, err)
			}
			if handlers.Status(err) != http.StatusBadRequest {
				t.Errorf("Status(ErrNotLocal) = %d, want 400", handlers.Status(err))
			}
			if rec.Flushed || len(rec.Header()) != 0 || rec.Body.Len() != 0 {
				t.Errorf("Redirect(%q) wrote to response: headers=%v body=%q", target, rec.Header(), rec.Body.String())
			}
		}
	})

	t.Run("nil target panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic on nil target")
			}
		}()
		_ = middleware.Redirect(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil), nil)
	})
}

func TestErrorPage(t *testing.T) {
	captureLogs := func(t *testing.T) *logCapture {
		t.Helper()
		orig := slog.Default()
		cap := &logCapture{}
		slog.SetDefault(slog.New(cap))
		t.Cleanup(func() { slog.SetDefault(orig) })
		return cap
	}

	t.Run("page gets handlers.Status(err) and writes whole answer", func(t *testing.T) {
		cap := captureLogs(t)
		var gotStatus int
		var gotErr error
		ew := middleware.ErrorPage(func(w http.ResponseWriter, r *http.Request, status int, err error) error {
			gotStatus = status
			gotErr = err
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "<h1>404 page</h1>")
			return nil
		})

		rec := httptest.NewRecorder()
		ew(rec, httptest.NewRequest("GET", "/missing", nil), errPageTest404)

		if gotStatus != 404 || !errors.Is(gotErr, errPageTest404) {
			t.Errorf("page got (%d, %v)", gotStatus, gotErr)
		}
		if rec.Code != 404 || rec.Body.String() != "<h1>404 page</h1>" {
			t.Errorf("response = (%d, %q)", rec.Code, rec.Body.String())
		}
		if len(cap.records) != 0 {
			t.Errorf("unexpected logs for 404: %v", cap.records)
		}
	})

	t.Run("page error before writing falls back to WriteText and logs both errors once each", func(t *testing.T) {
		cap := captureLogs(t)
		ew := middleware.ErrorPage(func(w http.ResponseWriter, r *http.Request, status int, err error) error {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Page", "stale")
			return stderrors.New("template render failed")
		})

		rec := httptest.NewRecorder()
		ew(rec, httptest.NewRequest("GET", "/boom", nil), stderrors.New("database down"))

		if rec.Code != 500 {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		if rec.Header().Get("X-Page") != "" {
			t.Errorf("stale header not cleared: %q", rec.Header().Get("X-Page"))
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("Content-Type = %q", ct)
		}
		if rec.Body.String() != "something went wrong" {
			t.Errorf("body = %q, want %q", rec.Body.String(), "something went wrong")
		}
		if len(cap.records) != 2 {
			t.Fatalf("expected 2 log records, got %d: %v", len(cap.records), cap.records)
		}
		if !strings.Contains(cap.records[0], "database down") {
			t.Errorf("first log = %q, want database down", cap.records[0])
		}
		if !strings.Contains(cap.records[1], "template render failed") {
			t.Errorf("second log = %q, want template render failed", cap.records[1])
		}
	})

	t.Run("page error after writing aborts with http.ErrAbortHandler", func(t *testing.T) {
		cap := captureLogs(t)
		ew := middleware.ErrorPage(func(w http.ResponseWriter, r *http.Request, status int, err error) error {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, "half")
			return stderrors.New("mid-stream failure")
		})

		var recovered any
		rec := httptest.NewRecorder()
		func() {
			defer func() { recovered = recover() }()
			ew(rec, httptest.NewRequest("GET", "/boom", nil), stderrors.New("database down"))
		}()

		if recovered != http.ErrAbortHandler {
			t.Errorf("recovered = %v, want http.ErrAbortHandler", recovered)
		}
		if len(cap.records) != 2 {
			t.Fatalf("expected 2 log records, got %d: %v", len(cap.records), cap.records)
		}
		if !strings.Contains(cap.records[0], "database down") || !strings.Contains(cap.records[1], "mid-stream failure") {
			t.Errorf("logs = %v", cap.records)
		}
	})
}

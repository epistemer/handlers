package middleware_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epistemer/handlers/middleware"
	"github.com/epistemer/i18n"
)

// seed returns the flash cookie a redirect writes after fn adds to it.
func seed(t *testing.T, fl *middleware.Flash, name string, fn func(r *http.Request)) *http.Cookie {
	t.Helper()
	h := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fn(r)
		w.WriteHeader(http.StatusSeeOther)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
	c := flashCookieFromResponse(rec.Result(), name)
	if c == nil {
		t.Fatal("no flash cookie")
	}
	return c
}

func serve(fl *middleware.Flash, c *http.Cookie, h http.HandlerFunc) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	if c != nil {
		req.AddCookie(c)
	}
	fl.Middleware(h).ServeHTTP(rec, req)
	return rec
}

func TestFlashHeldStatusIsSniffed(t *testing.T) {
	fl := middleware.NewFlash()
	c := seed(t, fl, "__Host-flash", func(r *http.Request) {
		fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("msg"))
	})

	// WriteHeader(404) before an HTML body with no Content-Type: delivered.
	rec := serve(fl, c, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, "<!doctype html><h1>missing</h1>")
	})
	if rec.Code != 404 {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	got := flashCookieFromResponse(rec.Result(), "__Host-flash")
	if got == nil || got.MaxAge >= 0 {
		t.Errorf("cookie = %+v, want it deleted (delivered)", got)
	}

	// WriteHeader(204) with no body: carried, no Set-Cookie.
	rec = serve(fl, c, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	if rec.Code != 204 || len(rec.Header().Values("Set-Cookie")) != 0 {
		t.Errorf("204: status %d, Set-Cookie %v; want 204 and none", rec.Code, rec.Header().Values("Set-Cookie"))
	}

	// An empty write doesn't decide: the HTML that follows delivers.
	rec = serve(fl, c, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(nil)
		_, _ = w.Write([]byte{})
		_, _ = io.WriteString(w, "<!doctype html><p>hi</p>")
	})
	if got := flashCookieFromResponse(rec.Result(), "__Host-flash"); got == nil || got.MaxAge >= 0 {
		t.Errorf("after empty writes: cookie = %+v, want it deleted", got)
	}
}

func TestFlashMessagesRoundTrip(t *testing.T) {
	fl := middleware.NewFlash()
	c := seed(t, fl, "__Host-flash", func(r *http.Request) {
		fl.Add(r.Context(), middleware.LevelSuccess, l10nSaved, "Confab")
	})
	// Read it back and add it again, as a message and as an argument.
	var next *http.Cookie
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for lvl, msg := range fl.Messages(r.Context()) {
			if _, isKer := msg.(i18n.Ker); isKer {
				t.Error("a message from Messages implements i18n.Ker")
			}
			fl.Add(r.Context(), lvl, msg)
			fl.Add(r.Context(), lvl, l10nWelcome, msg)
		}
		w.WriteHeader(http.StatusSeeOther)
	})
	rec := serve(fl, c, h)
	next = flashCookieFromResponse(rec.Result(), "__Host-flash")
	if next == nil {
		t.Fatal("no cookie after re-adding")
	}
	var texts []string
	serve(fl, next, func(w http.ResponseWriter, r *http.Request) {
		for _, m := range readMessages(r.Context(), fl) {
			texts = append(texts, m.text)
		}
	})
	want := []string{"Saved Confab", "Saved Confab", "Welcome Saved Confab"}
	if strings.Join(texts, "|") != strings.Join(want, "|") {
		t.Errorf("texts = %q, want %q", texts, want)
	}
}

func TestFlashOversizedAddKeepsOlder(t *testing.T) {
	fl := middleware.NewFlash()
	c := seed(t, fl, "__Host-flash", func(r *http.Request) {
		fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("small"))
	})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl.Add(r.Context(), middleware.LevelError, i18n.Static(strings.Repeat("x", 4000)))
		w.WriteHeader(http.StatusSeeOther)
	})
	rec := serve(fl, c, h)
	got := flashCookieFromResponse(rec.Result(), "__Host-flash")
	if got == nil || got.Value != c.Value {
		t.Errorf("cookie = %+v, want the older message only (%s)", got, c.Value)
	}
}

func TestFlashBadCookieIsCleared(t *testing.T) {
	fl := middleware.NewFlash()
	for name, value := range map[string]string{
		"malformed":     "!!!not-base64",
		"bad json":      base64.RawURLEncoding.EncodeToString([]byte("{")),
		"unknown level": base64.RawURLEncoding.EncodeToString([]byte(`[{"l":"loud","t":"x"}]`)),
		"over 4 KiB":    strings.Repeat("A", 5000),
	} {
		t.Run(name, func(t *testing.T) {
			rec := serve(fl, &http.Cookie{Name: "__Host-flash", Value: value}, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, "{}")
			})
			got := flashCookieFromResponse(rec.Result(), "__Host-flash")
			if rec.Code != 200 || got == nil || got.MaxAge >= 0 {
				t.Errorf("status %d, cookie %+v; want 200 and the cookie deleted", rec.Code, got)
			}
		})
	}
}

type emptyKer struct{}

func (emptyKer) K() string                        { return "" }
func (emptyKer) T(context.Context, ...any) string { return "plain" }

func TestFlashEmptyKeyIsText(t *testing.T) {
	fl := middleware.NewFlash()
	c := seed(t, fl, "__Host-flash", func(r *http.Request) {
		fl.Add(r.Context(), middleware.LevelInfo, emptyKer{})
	})
	var texts []string
	serve(fl, c, func(w http.ResponseWriter, r *http.Request) {
		for _, m := range readMessages(r.Context(), fl) {
			texts = append(texts, m.text)
		}
	})
	if len(texts) != 1 || texts[0] != "plain" {
		t.Errorf("texts = %q, want [plain]", texts)
	}
}

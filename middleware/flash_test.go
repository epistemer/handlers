package middleware_test

import (
	"bufio"
	"context"
	"encoding/base64"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers/middleware"
	"github.com/epistemer/i18n"
	"github.com/epistemer/i18n/i18ntest"
)

type eventID int64

var (
	l10nSaved      = i18n.Key("test.flash.saved", "Saved %s")
	l10nEventCount = i18n.Key("test.flash.event_count", "Event #%d (%t, %.1f)")
	l10nWelcome    = i18n.Key("test.flash.welcome", "Welcome %s")
	l10nOrderLabel = i18n.Key("test.flash.order_label", "Order")
	l10nErrWrapper = i18n.Key("test.flash.err_wrapper", "failed: %s")
	errTestTooHigh = errors.Key1[int]("test.flash.too_high", "limit %d exceeded", errors.Invalid)
)

type hijackSpy struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackSpy) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

type spy1xxWriter struct {
	header     http.Header
	statuses   []int
	body       strings.Builder
	wroteFinal bool
	setCookies []string
}

func newSpy1xxWriter() *spy1xxWriter {
	return &spy1xxWriter{header: make(http.Header)}
}

func (s *spy1xxWriter) Header() http.Header { return s.header }
func (s *spy1xxWriter) WriteHeader(code int) {
	s.statuses = append(s.statuses, code)
	if code >= 200 {
		s.wroteFinal = true
		s.setCookies = append([]string(nil), s.header.Values("Set-Cookie")...)
	}
}
func (s *spy1xxWriter) Write(p []byte) (int, error) {
	if !s.wroteFinal {
		s.WriteHeader(http.StatusOK)
	}
	return s.body.Write(p)
}

func flashCookieFromResponse(res *http.Response, name string) *http.Cookie {
	for _, c := range res.Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func readMessages(ctx context.Context, fl *middleware.Flash) []struct {
	level string
	text  string
} {
	var out []struct {
		level string
		text  string
	}
	for lvl, msg := range fl.Messages(ctx) {
		out = append(out, struct {
			level string
			text  string
		}{lvl, msg.T(ctx)})
	}
	return out
}

func TestFlashDeliveryFlows(t *testing.T) {
	t.Run("POST 303 GET delivers and clears cookie, Messages callable repeatedly", func(t *testing.T) {
		fl := middleware.NewFlash()
		mux := http.NewServeMux()
		mux.HandleFunc("POST /save", func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelSuccess, l10nSaved, "Confab")
			// Added in this request must not appear in Messages of this request.
			if got := readMessages(r.Context(), fl); len(got) != 0 {
				t.Errorf("Messages during POST = %+v, want empty", got)
			}
			_ = middleware.Redirect(w, r, stringLink("/done"))
		})
		mux.HandleFunc("GET /done", func(w http.ResponseWriter, r *http.Request) {
			m1 := readMessages(r.Context(), fl)
			m2 := readMessages(r.Context(), fl)
			if len(m1) != 1 || len(m2) != 1 || m1[0].level != middleware.LevelSuccess || m1[0].text != "Saved Confab" {
				t.Errorf("Messages = %+v / %+v", m1, m2)
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.WriteString(w, "<p>done</p>")
		})
		h := fl.Middleware(mux)

		// 1. POST -> 303 sets __Host-flash.
		rec1 := httptest.NewRecorder()
		h.ServeHTTP(rec1, httptest.NewRequest("POST", "/save", nil))
		if rec1.Code != http.StatusSeeOther {
			t.Fatalf("POST status = %d, want 303", rec1.Code)
		}
		c := flashCookieFromResponse(rec1.Result(), "__Host-flash")
		if c == nil || c.Value == "" {
			t.Fatalf("expected __Host-flash Set-Cookie on POST, got %v", rec1.Header().Values("Set-Cookie"))
		}

		// 2. GET -> 200 text/html delivers and clears cookie.
		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("GET", "/done", nil)
		req2.AddCookie(c)
		h.ServeHTTP(rec2, req2)
		if rec2.Code != http.StatusOK {
			t.Fatalf("GET status = %d", rec2.Code)
		}
		cleared := flashCookieFromResponse(rec2.Result(), "__Host-flash")
		if cleared == nil || cleared.MaxAge >= 0 || cleared.Value != "" {
			t.Errorf("expected cleared cookie on GET, got %+v", cleared)
		}
	})

	t.Run("chain of two redirects preserves cookie until final HTML page", func(t *testing.T) {
		fl := middleware.NewFlash()
		mux := http.NewServeMux()
		mux.HandleFunc("POST /save", func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("hello"))
			_ = middleware.Redirect(w, r, stringLink("/middle"))
		})
		mux.HandleFunc("GET /middle", func(w http.ResponseWriter, r *http.Request) {
			// http.Redirect on GET sets text/html + 303: must NOT deliver.
			_ = middleware.Redirect(w, r, stringLink("/login"))
		})
		mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
			msgs := readMessages(r.Context(), fl)
			if len(msgs) != 1 || msgs[0].text != "hello" {
				t.Errorf("login Messages = %+v", msgs)
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "login")
		})
		h := fl.Middleware(mux)

		rec1 := httptest.NewRecorder()
		h.ServeHTTP(rec1, httptest.NewRequest("POST", "/save", nil))
		c := flashCookieFromResponse(rec1.Result(), "__Host-flash")
		if c == nil {
			t.Fatal("missing cookie after POST")
		}

		rec2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("GET", "/middle", nil)
		req2.AddCookie(c)
		h.ServeHTTP(rec2, req2)
		if rec2.Code != 303 {
			t.Fatalf("middle status = %d", rec2.Code)
		}
		if len(rec2.Header().Values("Set-Cookie")) != 0 {
			t.Errorf("redirect should not touch Set-Cookie, got %v", rec2.Header().Values("Set-Cookie"))
		}

		rec3 := httptest.NewRecorder()
		req3 := httptest.NewRequest("GET", "/login", nil)
		req3.AddCookie(c)
		h.ServeHTTP(rec3, req3)
		if cleared := flashCookieFromResponse(rec3.Result(), "__Host-flash"); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("expected cookie cleared after final HTML page, got %+v", cleared)
		}
	})

	t.Run("HX-Redirect preserves flash and non-boosted htmx fragment leaves cookie untouched", func(t *testing.T) {
		fl := middleware.NewFlash()
		mux := http.NewServeMux()
		mux.HandleFunc("POST /action", func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelWarning, i18n.Static("careful"))
			_ = middleware.Redirect(w, r, stringLink("/next"))
		})
		mux.HandleFunc("GET /frag", func(w http.ResponseWriter, r *http.Request) {
			if got := readMessages(r.Context(), fl); len(got) != 0 {
				t.Errorf("non-boosted htmx should receive no messages, got %+v", got)
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "<div>frag</div>")
		})
		mux.HandleFunc("GET /next", func(w http.ResponseWriter, r *http.Request) {
			msgs := readMessages(r.Context(), fl)
			if len(msgs) != 1 || msgs[0].level != middleware.LevelWarning || msgs[0].text != "careful" {
				t.Errorf("/next Messages = %+v", msgs)
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "next")
		})
		h := fl.Middleware(mux)

		// htmx POST -> 204 HX-Redirect sets cookie.
		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest("POST", "/action", nil)
		req1.Header.Set("HX-Request", "true")
		h.ServeHTTP(rec1, req1)
		if rec1.Code != http.StatusNoContent || rec1.Header().Get("HX-Redirect") != "/next" {
			t.Fatalf("POST htmx = (%d, %q)", rec1.Code, rec1.Header().Get("HX-Redirect"))
		}
		c := flashCookieFromResponse(rec1.Result(), "__Host-flash")
		if c == nil {
			t.Fatal("missing flash cookie on HX-Redirect")
		}

		// Non-boosted htmx fragment -> 200 text/html -> NOT delivered, NO Set-Cookie.
		recFrag := httptest.NewRecorder()
		reqFrag := httptest.NewRequest("GET", "/frag", nil)
		reqFrag.Header.Set("HX-Request", "true")
		reqFrag.AddCookie(c)
		h.ServeHTTP(recFrag, reqFrag)
		if len(recFrag.Header().Values("Set-Cookie")) != 0 {
			t.Errorf("non-boosted fragment wrote Set-Cookie: %v", recFrag.Header().Values("Set-Cookie"))
		}

		// Boosted htmx request -> IS delivered and clears cookie.
		recBoost := httptest.NewRecorder()
		reqBoost := httptest.NewRequest("GET", "/next", nil)
		reqBoost.Header.Set("HX-Request", "true")
		reqBoost.Header.Set("HX-Boosted", "true")
		reqBoost.AddCookie(c)
		h.ServeHTTP(recBoost, reqBoost)
		if cleared := flashCookieFromResponse(recBoost.Result(), "__Host-flash"); cleared == nil || cleared.MaxAge >= 0 {
			t.Errorf("boosted request should clear cookie, got %+v", cleared)
		}
	})

	t.Run("HEAD, JSON, text/plain 404 do not deliver; sniffed HTML delivers", func(t *testing.T) {
		fl := middleware.NewFlash()
		seedH := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("msg"))
			w.WriteHeader(http.StatusSeeOther)
		}))
		seedRec := httptest.NewRecorder()
		seedH.ServeHTTP(seedRec, httptest.NewRequest("POST", "/", nil))
		cookie := flashCookieFromResponse(seedRec.Result(), "__Host-flash")
		if cookie == nil {
			t.Fatal("expected seed cookie")
		}

		cases := []struct {
			name        string
			method      string
			ct          string
			status      int
			body        string
			wantDeliver bool
		}{
			{"HEAD with text/html", "HEAD", "text/html", 200, "", false},
			{"JSON 200", "GET", "application/json", 200, `{}`, false},
			{"text/plain 404", "GET", "text/plain; charset=utf-8", 404, "not found", false},
			{"HTML 404", "GET", "text/html; charset=utf-8", 404, "<h1>404</h1>", true},
			{"sniffed HTML without Content-Type", "GET", "", 200, "<!doctype html><h1>hi</h1>", true},
			{"sniffed plain text without Content-Type", "GET", "", 200, "just text", false},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				var msgsLen int
				h := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					msgsLen = len(readMessages(r.Context(), fl))
					if tc.ct != "" {
						w.Header().Set("Content-Type", tc.ct)
					}
					if tc.status != 200 || tc.body == "" {
						w.WriteHeader(tc.status)
					}
					if tc.body != "" {
						_, _ = io.WriteString(w, tc.body)
					}
				}))
				rec := httptest.NewRecorder()
				req := httptest.NewRequest(tc.method, "/", nil)
				req.AddCookie(cookie)
				h.ServeHTTP(rec, req)

				hasSetCookie := len(rec.Header().Values("Set-Cookie")) > 0
				if hasSetCookie != tc.wantDeliver {
					t.Errorf("Set-Cookie present = %v, want %v (headers=%v)", hasSetCookie, tc.wantDeliver, rec.Header())
				}
				if tc.method == "HEAD" && msgsLen != 0 {
					t.Errorf("HEAD should see 0 messages, got %d", msgsLen)
				}
			})
		}
	})

	t.Run("5xx drops adds and no Set-Cookie when nothing changed", func(t *testing.T) {
		fl := middleware.NewFlash()
		h := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelSuccess, i18n.Static("saved"))
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))
		if len(rec.Header().Values("Set-Cookie")) != 0 {
			t.Errorf("500 must drop adds and emit no Set-Cookie, got %v", rec.Header().Values("Set-Cookie"))
		}

		// Handler returns without writing and without Add -> no Set-Cookie.
		hNoop := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		recNoop := httptest.NewRecorder()
		hNoop.ServeHTTP(recNoop, httptest.NewRequest("GET", "/", nil))
		if len(recNoop.Header().Values("Set-Cookie")) != 0 {
			t.Errorf("noop handler emitted Set-Cookie: %v", recNoop.Header().Values("Set-Cookie"))
		}

		// Handler returns without writing WITH Add -> Set-Cookie written.
		hAddNoop := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("queued"))
		}))
		recAddNoop := httptest.NewRecorder()
		hAddNoop.ServeHTTP(recAddNoop, httptest.NewRequest("POST", "/", nil))
		if c := flashCookieFromResponse(recAddNoop.Result(), "__Host-flash"); c == nil || c.Value == "" {
			t.Errorf("expected Set-Cookie when Add called without Write")
		}
	})

	t.Run("Flush before WriteHeader triggers decision and 1xx is ignored", func(t *testing.T) {
		fl := middleware.NewFlash()

		// Flush before WriteHeader with Add.
		hFlush := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("flushed"))
			w.(http.Flusher).Flush()
		}))
		recFlush := httptest.NewRecorder()
		hFlush.ServeHTTP(recFlush, httptest.NewRequest("GET", "/", nil))
		if !recFlush.Flushed || flashCookieFromResponse(recFlush.Result(), "__Host-flash") == nil {
			t.Errorf("Flush before WriteHeader: flushed=%v, cookies=%v", recFlush.Flushed, recFlush.Header().Values("Set-Cookie"))
		}

		// 1xx ignored: 103 Early Hints does not decide early; subsequent 200 text/html delivers.
		seedRec := httptest.NewRecorder()
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("early"))
		})).ServeHTTP(seedRec, httptest.NewRequest("POST", "/", nil))
		c := flashCookieFromResponse(seedRec.Result(), "__Host-flash")

		spy := newSpy1xxWriter()
		h1xx := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusEarlyHints)
			if len(spy.Header().Values("Set-Cookie")) != 0 {
				t.Errorf("1xx must not trigger Set-Cookie decision yet")
			}
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "ok")
		}))
		req1xx := httptest.NewRequest("GET", "/", nil)
		req1xx.AddCookie(c)
		h1xx.ServeHTTP(spy, req1xx)
		if len(spy.statuses) != 2 || spy.statuses[0] != 103 || spy.statuses[1] != 200 {
			t.Errorf("statuses = %v, want [103 200]", spy.statuses)
		}
		if len(spy.setCookies) != 1 || !strings.Contains(spy.setCookies[0], "Max-Age=0") {
			t.Errorf("expected cleared cookie on final 200, got %v", spy.setCookies)
		}

		// Nothing decided/written after a hijack.
		hjSpy := &hijackSpy{ResponseRecorder: httptest.NewRecorder()}
		hHj := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("after"))
			if _, _, err := w.(http.Hijacker).Hijack(); err != nil {
				t.Fatal(err)
			}
		}))
		reqHj := httptest.NewRequest("GET", "/", nil)
		reqHj.AddCookie(c)
		hHj.ServeHTTP(hjSpy, reqHj)
		if !hjSpy.hijacked || len(hjSpy.Header().Values("Set-Cookie")) != 0 {
			t.Errorf("hijack: hijacked=%v cookies=%v", hjSpy.hijacked, hjSpy.Header().Values("Set-Cookie"))
		}
	})
}

func TestFlashCookieAttributesAndLimits(t *testing.T) {
	orig := slog.Default()
	cap := &logCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(orig) })

	t.Run("cookie attributes default vs FlashInsecure and FlashName", func(t *testing.T) {
		flSec := middleware.NewFlash(middleware.FlashName("notice"))
		rec1 := httptest.NewRecorder()
		flSec.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flSec.Add(r.Context(), middleware.LevelInfo, i18n.Static("hi"))
			w.WriteHeader(http.StatusSeeOther)
		})).ServeHTTP(rec1, httptest.NewRequest("POST", "/", nil))

		c1 := flashCookieFromResponse(rec1.Result(), "__Host-notice")
		if c1 == nil {
			t.Fatalf("missing __Host-notice cookie, got %v", rec1.Header().Values("Set-Cookie"))
		}
		if !c1.Secure || !c1.HttpOnly || c1.SameSite != http.SameSiteLaxMode || c1.Path != "/" || c1.MaxAge != 300 {
			t.Errorf("secure cookie attrs = %+v", c1)
		}

		flInsec := middleware.NewFlash(middleware.FlashName("notice"), middleware.FlashInsecure())
		rec2 := httptest.NewRecorder()
		flInsec.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			flInsec.Add(r.Context(), middleware.LevelInfo, i18n.Static("hi"))
			w.WriteHeader(http.StatusSeeOther)
		})).ServeHTTP(rec2, httptest.NewRequest("POST", "/", nil))

		c2 := flashCookieFromResponse(rec2.Result(), "notice")
		if c2 == nil {
			t.Fatalf("missing notice cookie, got %v", rec2.Header().Values("Set-Cookie"))
		}
		if c2.Secure || !c2.HttpOnly || c2.SameSite != http.SameSiteLaxMode || c2.Path != "/" || c2.MaxAge != 300 {
			t.Errorf("insecure cookie attrs = %+v", c2)
		}
	})

	t.Run("cookie over 4 KiB ignored undecoded and malformed/unknown dropped at debug", func(t *testing.T) {
		fl := middleware.NewFlash()
		badValues := []struct {
			name string
			val  string
		}{
			{"over 4 KiB", strings.Repeat("a", 4097)},
			{"bad base64", "!!!not-base64!!!"},
			{"bad json", base64.RawURLEncoding.EncodeToString([]byte(`{not json`))},
			{"unknown level", base64.RawURLEncoding.EncodeToString([]byte(`[{"l":"debug","t":"hi"}]`))},
			{"unknown arg tag", base64.RawURLEncoding.EncodeToString([]byte(`[{"l":"info","k":"test.flash.saved","a":[{"t":"ptr","v":"x"}]}]`))},
		}
		for _, tc := range badValues {
			cap.records = nil
			var count int
			h := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				count = len(readMessages(r.Context(), fl))
				w.Header().Set("Content-Type", "text/html")
				_, _ = io.WriteString(w, "ok")
			}))
			rec := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.AddCookie(&http.Cookie{Name: "__Host-flash", Value: tc.val})
			h.ServeHTTP(rec, req)

			if rec.Code != 200 || count != 0 {
				t.Errorf("%s: status=%d count=%d, want (200, 0)", tc.name, rec.Code, count)
			}
			if len(cap.records) == 0 || !strings.HasPrefix(cap.records[0], "DEBUG ") {
				t.Errorf("%s: expected DEBUG log, got %v", tc.name, cap.records)
			}
		}
	})

	t.Run("over 3 KiB written drops oldest with warn log", func(t *testing.T) {
		cap.records = nil
		fl := middleware.NewFlash()
		h := fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("oldest-"+strings.Repeat("a", 1500)))
			fl.Add(r.Context(), middleware.LevelInfo, i18n.Static("middle-"+strings.Repeat("b", 1500)))
			fl.Add(r.Context(), middleware.LevelSuccess, i18n.Static("newest"))
			w.WriteHeader(http.StatusSeeOther)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("POST", "/", nil))

		c := flashCookieFromResponse(rec.Result(), "__Host-flash")
		if c == nil || len(c.Value) > 3072 {
			t.Fatalf("expected cookie <= 3072 bytes, got %v", c)
		}
		if len(cap.records) == 0 || !strings.HasPrefix(cap.records[0], "WARN ") {
			t.Errorf("expected WARN log, got %v", cap.records)
		}

		// Read back: oldest was dropped, newest survived.
		var got []string
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, m := range readMessages(r.Context(), fl) {
				got = append(got, m.text)
			}
		})).ServeHTTP(httptest.NewRecorder(), func() *http.Request {
			r := httptest.NewRequest("GET", "/", nil)
			r.AddCookie(c)
			return r
		}())
		if len(got) == 0 || got[len(got)-1] != "newest" || strings.HasPrefix(got[0], "oldest-") {
			t.Errorf("surviving messages = %v", got)
		}
	})
}

func TestFlashContentTypesAndLocales(t *testing.T) {
	tr := i18ntest.New(t, i18n.Locale{Code: "en", NativeName: "English"})
	if err := tr.CreateLocale(i18n.Locale{Code: "ar"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("ar", l10nWelcome.K(), "أهلاً %s"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("ar", l10nOrderLabel.K(), "الطلب"); err != nil {
		t.Fatal(err)
	}

	t.Run("defined int type renders with %d after round trip", func(t *testing.T) {
		fl := middleware.NewFlash()
		rec1 := httptest.NewRecorder()
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelInfo, l10nEventCount, eventID(42), true, 3.5)
			w.WriteHeader(http.StatusSeeOther)
		})).ServeHTTP(rec1, httptest.NewRequest("POST", "/", nil))
		c := flashCookieFromResponse(rec1.Result(), "__Host-flash")

		var got string
		req2 := httptest.NewRequest("GET", "/", nil)
		req2.AddCookie(c)
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = readMessages(r.Context(), fl)[0].text
		})).ServeHTTP(httptest.NewRecorder(), req2)

		if got != "Event #42 (true, 3.5)" {
			t.Errorf("got %q, want %q", got, "Event #42 (true, 3.5)")
		}
	})

	t.Run("errors tested before keys as msg and as arg", func(t *testing.T) {
		fl := middleware.NewFlash()
		occ := errTestTooHigh.With(5)
		rec1 := httptest.NewRecorder()
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelError, occ.(i18n.Ter))
			fl.Add(r.Context(), middleware.LevelError, l10nErrWrapper, occ)
			w.WriteHeader(http.StatusSeeOther)
		})).ServeHTTP(rec1, httptest.NewRequest("POST", "/", nil))
		c := flashCookieFromResponse(rec1.Result(), "__Host-flash")

		var msgs []string
		req2 := httptest.NewRequest("GET", "/", nil)
		req2.AddCookie(c)
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, m := range readMessages(r.Context(), fl) {
				msgs = append(msgs, m.text)
			}
		})).ServeHTTP(httptest.NewRecorder(), req2)

		if len(msgs) != 2 || msgs[0] != "limit 5 exceeded" || msgs[1] != "failed: limit 5 exceeded" {
			t.Errorf("msgs = %v, want [\"limit 5 exceeded\", \"failed: limit 5 exceeded\"]", msgs)
		}
	})

	t.Run("key and key arg added in en render in reader locale ar", func(t *testing.T) {
		fl := middleware.NewFlash()
		rec1 := httptest.NewRecorder()
		req1 := httptest.NewRequest("POST", "/", nil).WithContext(i18ntest.Context(t, tr, "en"))
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl.Add(r.Context(), middleware.LevelSuccess, l10nWelcome, l10nOrderLabel)
			w.WriteHeader(http.StatusSeeOther)
		})).ServeHTTP(rec1, req1)
		c := flashCookieFromResponse(rec1.Result(), "__Host-flash")

		var got string
		req2 := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "ar"))
		req2.AddCookie(c)
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = readMessages(r.Context(), fl)[0].text
		})).ServeHTTP(httptest.NewRecorder(), req2)

		if got != "أهلاً الطلب" {
			t.Errorf("got %q, want %q", got, "أهلاً الطلب")
		}
	})
}

func TestFlashPanicsAndIsolation(t *testing.T) {
	fl := middleware.NewFlash()

	t.Run("Add outside middleware panics", func(t *testing.T) {
		var got any
		func() {
			defer func() { got = recover() }()
			fl.Add(context.Background(), middleware.LevelInfo, i18n.Static("x"))
		}()
		if got != "middleware: Flash.Add outside its Middleware" {
			t.Errorf("panic = %v, want %q", got, "middleware: Flash.Add outside its Middleware")
		}
	})

	t.Run("Add with unknown level panics", func(t *testing.T) {
		var got any
		fl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() { got = recover() }()
			fl.Add(r.Context(), "notice", i18n.Static("x"))
		})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))

		want := `middleware: unknown flash level "notice"`
		if got != want {
			t.Errorf("panic = %v, want %q", got, want)
		}
	})

	t.Run("two Flash values in one app do not share state", func(t *testing.T) {
		fl1 := middleware.NewFlash(middleware.FlashName("one"))
		fl2 := middleware.NewFlash(middleware.FlashName("two"))

		var got1, got2 int
		h := fl1.Middleware(fl2.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl1.Add(r.Context(), middleware.LevelInfo, i18n.Static("only1"))
			for range fl1.Messages(r.Context()) {
				got1++
			}
			for range fl2.Messages(r.Context()) {
				got2++
			}
			w.Header().Set("Content-Type", "text/html")
			_, _ = io.WriteString(w, "ok")
		})))

		// Seed fl1 only.
		seedRec := httptest.NewRecorder()
		fl1.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fl1.Add(r.Context(), middleware.LevelInfo, i18n.Static("from1"))
		})).ServeHTTP(seedRec, httptest.NewRequest("POST", "/", nil))
		c1 := flashCookieFromResponse(seedRec.Result(), "__Host-one")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(c1)
		h.ServeHTTP(rec, req)

		if got1 != 1 || got2 != 0 {
			t.Errorf("got1=%d got2=%d, want 1 and 0", got1, got2)
		}
		if flashCookieFromResponse(rec.Result(), "__Host-two") != nil {
			t.Errorf("fl2 should not have written a cookie")
		}
	})
}

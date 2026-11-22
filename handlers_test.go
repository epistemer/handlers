package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/epistemer/i18n"
	"github.com/epistemer/i18n/i18ntest"
)

var (
	testParamLabel   = i18n.Key("test.param.id", "Order ID")
	testRouteLabel   = i18n.Key("test.route.home", "Home")
	errTestUnauth    = errors.Key("test.unauth", "unauth", errors.Unauthenticated)
	errTestForbid    = errors.Key("test.forbid", "forbidden", errors.Forbidden)
	errTestConflict  = errors.Key("test.conflict", "conflict", errors.Conflict)
	errTestRate      = errors.Key("test.rate", "slow down", errors.RateLimited)
	errTestUnavail   = errors.Key("test.unavail", "down", errors.Unavailable)
	errTestInternal  = errors.Key("test.internal", "oops", errors.Internal)
	errTestGuardDeny = errors.Key("test.guard.deny", "denied", errors.Forbidden)
)

type fakeGuard struct {
	prepareErr error
	checkErr   error
	prepared   []handlers.Route
	checked    []handlers.Route
	sawRoute   bool
}

func (g *fakeGuard) Prepare(rt handlers.Route) error {
	g.prepared = append(g.prepared, rt)
	return g.prepareErr
}

func (g *fakeGuard) Check(r *http.Request, rt handlers.Route) error {
	g.checked = append(g.checked, rt)
	if got, ok := handlers.From(r.Context()); ok && got.Key == rt.Key {
		g.sawRoute = true
	}
	return g.checkErr
}

func (g *fakeGuard) Admit(*http.Request) (func(handlers.Route) bool, error) {
	return func(handlers.Route) bool { return true }, nil
}

type textParam struct {
	v string
}

func (t *textParam) UnmarshalText(b []byte) error {
	if string(b) == "bad" {
		return stderrors.New("bad text")
	}
	t.v = strings.ToUpper(string(b))
	return nil
}

func (t textParam) MarshalText() ([]byte, error) {
	if t.v == "fail" {
		return nil, stderrors.New("marshal failed")
	}
	return []byte(strings.ToLower(t.v)), nil
}

func assertPanics(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s: expected panic, got none", name)
		}
	}()
	fn()
}

func TestSurfaceAndAuthString(t *testing.T) {
	if got := handlers.HTML.String(); got != "html" {
		t.Errorf("HTML.String() = %q, want html", got)
	}
	if got := handlers.JSON.String(); got != "json" {
		t.Errorf("JSON.String() = %q, want json", got)
	}
	assertPanics(t, "Surface(0).String", func() { _ = handlers.Surface(0).String() })
	assertPanics(t, "Surface(99).String", func() { _ = handlers.Surface(99).String() })

	if got := handlers.Public.String(); got != "public" {
		t.Errorf("Public.String() = %q, want public", got)
	}
	if got := handlers.Session.String(); got != "session" {
		t.Errorf("Session.String() = %q, want session", got)
	}
	if got := handlers.Permission.String(); got != "permission" {
		t.Errorf("Permission.String() = %q, want permission", got)
	}
	assertPanics(t, "Auth(0).String", func() { _ = handlers.Auth(0).String() })
	assertPanics(t, "Auth(99).String", func() { _ = handlers.Auth(99).String() })
}

func TestFromAndWithRoute(t *testing.T) {
	var nilCtx context.Context
	if _, ok := handlers.From(nilCtx); ok {
		t.Error("From(nil) = true, want false")
	}
	if _, ok := handlers.From(context.Background()); ok {
		t.Error("From(Background) = true, want false")
	}
	want := handlers.Route{Key: "home", Method: "GET", Pattern: "/"}
	ctx := handlers.WithRoute(context.Background(), want)
	got, ok := handlers.From(ctx)
	if !ok || got.Key != want.Key || got.Method != want.Method {
		t.Errorf("From(WithRoute) = (%+v, %v), want (%+v, true)", got, ok, want)
	}
}

func TestStatus(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"ErrTooLarge", handlers.ErrTooLarge, http.StatusRequestEntityTooLarge},
		{"wrapped ErrTooLarge", fmt.Errorf("body: %w", handlers.ErrTooLarge), http.StatusRequestEntityTooLarge},
		{"MaxBytesError", &http.MaxBytesError{Limit: 10}, http.StatusRequestEntityTooLarge},
		{"wrapped MaxBytesError", fmt.Errorf("read: %w", &http.MaxBytesError{Limit: 10}), http.StatusRequestEntityTooLarge},
		{"ErrBadParam", handlers.ErrBadParam.With(testParamLabel), http.StatusBadRequest},
		{"ErrInvalidRequest", handlers.ErrInvalidRequest, http.StatusBadRequest},
		{"ErrInvalidField", handlers.ErrInvalidField.With("name"), http.StatusBadRequest},
		{"ErrNotFound", handlers.ErrNotFound, http.StatusNotFound},
		{"Unauthenticated", errTestUnauth, http.StatusUnauthorized},
		{"Forbidden", errTestForbid, http.StatusForbidden},
		{"Conflict", errTestConflict, http.StatusConflict},
		{"RateLimited", errTestRate, http.StatusTooManyRequests},
		{"Unavailable", errTestUnavail, http.StatusServiceUnavailable},
		{"Internal", errTestInternal, http.StatusInternalServerError},
		{"plain error", stderrors.New("secret"), http.StatusInternalServerError},
		{"nil", nil, http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := handlers.Status(tc.err); got != tc.want {
				t.Errorf("Status(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

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

func TestWriteTextAndWriteJSON(t *testing.T) {
	tr := i18ntest.New(t, i18n.Locale{Code: "en", NativeName: "English"})
	if err := tr.CreateLocale(i18n.Locale{Code: "ar"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("ar", handlers.ErrNotFound.K(), "غير موجود"); err != nil {
		t.Fatal(err)
	}

	orig := slog.Default()
	cap := &logCapture{}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(orig) })

	t.Run("WriteText 404 localized not logged", func(t *testing.T) {
		cap.records = nil
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil).WithContext(i18ntest.Context(t, tr, "ar"))
		handlers.WriteText(rec, req, handlers.ErrNotFound)

		if rec.Code != 404 {
			t.Errorf("status = %d, want 404", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("Content-Type = %q", ct)
		}
		if rec.Body.String() != "غير موجود" {
			t.Errorf("body = %q", rec.Body.String())
		}
		if len(cap.records) != 0 {
			t.Errorf("unexpected log records for 404: %v", cap.records)
		}
	})

	t.Run("WriteText 500 hides internal cause and logs", func(t *testing.T) {
		cap.records = nil
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		handlers.WriteText(rec, req, stderrors.New("db password leaked"))

		if rec.Code != 500 {
			t.Errorf("status = %d, want 500", rec.Code)
		}
		if strings.Contains(rec.Body.String(), "password") {
			t.Errorf("body leaked internal error: %q", rec.Body.String())
		}
		if rec.Body.String() != "something went wrong" {
			t.Errorf("body = %q, want %q", rec.Body.String(), "something went wrong")
		}
		if len(cap.records) != 1 || !strings.Contains(cap.records[0], "db password leaked") {
			t.Errorf("expected slog record with err.Error(), got %v", cap.records)
		}
	})

	t.Run("WriteJSON 400 and 500", func(t *testing.T) {
		cap.records = nil
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", nil)
		handlers.WriteJSON(rec, req, handlers.ErrInvalidField.With("/email"))

		if rec.Code != 400 {
			t.Errorf("status = %d, want 400", rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q", ct)
		}
		var body struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != "errors.handlers.invalid_field" || body.Message != "the request is not valid at /email" {
			t.Errorf("body = %+v", body)
		}
		if len(cap.records) != 0 {
			t.Errorf("unexpected log for 400: %v", cap.records)
		}

		// 500
		rec2 := httptest.NewRecorder()
		handlers.WriteJSON(rec2, req, fmt.Errorf("sql: %w", stderrors.New("connection refused")))
		if rec2.Code != 500 {
			t.Errorf("status = %d, want 500", rec2.Code)
		}
		var body2 struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec2.Body.Bytes(), &body2); err != nil {
			t.Fatal(err)
		}
		if body2.Code != "errors.unknown" || body2.Message != "something went wrong" {
			t.Errorf("body2 = %+v", body2)
		}
		if len(cap.records) != 1 || !strings.Contains(cap.records[0], "connection refused") {
			t.Errorf("expected slog record with connection refused, got %v", cap.records)
		}
	})
}

func TestParamHelpers(t *testing.T) {
	reqWith := func(name, val string) *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.SetPathValue(name, val)
		return r
	}
	p := handlers.Param{Name: "id", Label: testParamLabel}

	t.Run("ParamString", func(t *testing.T) {
		if got := handlers.ParamString(reqWith("id", "hello/world"), p); got != "hello/world" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("ParamInt widths and errors", func(t *testing.T) {
		valid := []struct {
			bits int
			in   string
			want int64
		}{
			{0, "-42", -42},
			{8, "-128", -128},
			{8, "127", 127},
			{16, "-32768", -32768},
			{16, "32767", 32767},
			{32, "-2147483648", -2147483648},
			{32, "2147483647", 2147483647},
			{64, "-9223372036854775808", -9223372036854775808},
			{64, "9223372036854775807", 9223372036854775807},
		}
		for _, tc := range valid {
			got, err := handlers.ParamInt(reqWith("id", tc.in), p, tc.bits)
			if err != nil || got != tc.want {
				t.Errorf("ParamInt(%q, %d) = (%d, %v), want (%d, nil)", tc.in, tc.bits, got, err, tc.want)
			}
		}

		invalid := []struct {
			bits int
			in   string
		}{
			{0, "abc"},
			{0, ""},
			{8, "128"},
			{8, "-129"},
			{16, "32768"},
			{32, "2147483648"},
			{64, "9223372036854775808"},
		}
		for _, tc := range invalid {
			_, err := handlers.ParamInt(reqWith("id", tc.in), p, tc.bits)
			if !errors.Is(err, handlers.ErrBadParam) {
				t.Errorf("ParamInt(%q, %d) err = %v, want ErrBadParam", tc.in, tc.bits, err)
			}
			if got := errors.T(context.Background(), err); got != "Order ID is not valid" {
				t.Errorf("errors.T = %q, want %q", got, "Order ID is not valid")
			}
		}
	})

	t.Run("ParamUint widths and errors", func(t *testing.T) {
		valid := []struct {
			bits int
			in   string
			want uint64
		}{
			{0, "42", 42},
			{8, "255", 255},
			{16, "65535", 65535},
			{32, "4294967295", 4294967295},
			{64, "18446744073709551615", 18446744073709551615},
		}
		for _, tc := range valid {
			got, err := handlers.ParamUint(reqWith("id", tc.in), p, tc.bits)
			if err != nil || got != tc.want {
				t.Errorf("ParamUint(%q, %d) = (%d, %v), want (%d, nil)", tc.in, tc.bits, got, err, tc.want)
			}
		}

		invalid := []struct {
			bits int
			in   string
		}{
			{0, "-1"},
			{0, "xyz"},
			{8, "256"},
			{16, "65536"},
			{32, "4294967296"},
			{64, "18446744073709551616"},
		}
		for _, tc := range invalid {
			_, err := handlers.ParamUint(reqWith("id", tc.in), p, tc.bits)
			if !errors.Is(err, handlers.ErrBadParam) {
				t.Errorf("ParamUint(%q, %d) err = %v, want ErrBadParam", tc.in, tc.bits, err)
			}
		}
	})

	t.Run("ParamBool", func(t *testing.T) {
		for _, tc := range []struct {
			in   string
			want bool
		}{
			{"true", true},
			{"1", true},
			{"false", false},
			{"0", false},
		} {
			got, err := handlers.ParamBool(reqWith("id", tc.in), p)
			if err != nil || got != tc.want {
				t.Errorf("ParamBool(%q) = (%v, %v), want (%v, nil)", tc.in, got, err, tc.want)
			}
		}
		if _, err := handlers.ParamBool(reqWith("id", "maybe"), p); !errors.Is(err, handlers.ErrBadParam) {
			t.Errorf("ParamBool(maybe) err = %v, want ErrBadParam", err)
		}
	})

	t.Run("ParamText", func(t *testing.T) {
		var dst textParam
		if err := handlers.ParamText(reqWith("id", "ok"), p, &dst); err != nil || dst.v != "OK" {
			t.Errorf("ParamText(ok) = (%q, %v)", dst.v, err)
		}
		if err := handlers.ParamText(reqWith("id", "bad"), p, &dst); !errors.Is(err, handlers.ErrBadParam) {
			t.Errorf("ParamText(bad) err = %v, want ErrBadParam", err)
		}
	})
}

func TestSegmentRestText(t *testing.T) {
	if got := handlers.Segment("a b/c?d#e"); got != "a%20b%2Fc%3Fd%23e" {
		t.Errorf("Segment = %q", got)
	}
	if got := handlers.Rest("dir/sub dir/file?1"); got != "dir/sub%20dir/file%3F1" {
		t.Errorf("Rest = %q", got)
	}
	if got := handlers.Rest(""); got != "" {
		t.Errorf("Rest(\"\") = %q", got)
	}
	if got := handlers.Text(textParam{v: "ABC"}); got != "abc" {
		t.Errorf("Text = %q", got)
	}
	assertPanics(t, "Text panic on error", func() {
		_ = handlers.Text(textParam{v: "fail"})
	})
}

func TestMountPanics(t *testing.T) {
	assertPanics(t, "nil mux", func() {
		handlers.NewMount(nil, nil, nil)
	})
	assertPanics(t, "negative max body", func() {
		handlers.WithMaxBody(-1)
	})
	assertPanics(t, "nil middleware", func() {
		handlers.WithMiddleware(nil)
	})

	pub := handlers.Route{Key: "pub", Method: "GET", Pattern: "/", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel}
	sess := handlers.Route{Key: "sess", Method: "GET", Pattern: "/s", Surface: handlers.HTML, Auth: handlers.Session, Label: testRouteLabel}
	noop := func(http.ResponseWriter, *http.Request) error { return nil }

	assertPanics(t, "nil guard on non-public route", func() {
		m := handlers.NewMount(http.NewServeMux(), []handlers.Route{sess}, nil)
		m.Handle(0, noop)
	})

	assertPanics(t, "Prepare error", func() {
		g := &fakeGuard{prepareErr: stderrors.New("undeclared perm")}
		m := handlers.NewMount(http.NewServeMux(), []handlers.Route{sess}, g)
		m.Handle(0, noop)
	})

	assertPanics(t, "unknown Auth", func() {
		bad := pub
		bad.Auth = handlers.Auth(99)
		m := handlers.NewMount(http.NewServeMux(), []handlers.Route{bad}, &fakeGuard{})
		m.Handle(0, noop)
	})

	assertPanics(t, "unknown Surface", func() {
		bad := pub
		bad.Surface = handlers.Surface(99)
		m := handlers.NewMount(http.NewServeMux(), []handlers.Route{bad}, nil)
		m.Handle(0, noop)
	})

	assertPanics(t, "Done with missing route", func() {
		m := handlers.NewMount(http.NewServeMux(), []handlers.Route{pub}, nil)
		m.Done()
	})

	// Public route skips guard even when guard is nil.
	mux := http.NewServeMux()
	m := handlers.NewMount(mux, []handlers.Route{pub}, nil)
	m.Handle(0, noop)
	m.Done()
}

func TestMountChainAndRouting(t *testing.T) {
	t.Run("chain order, Prepare once, public skips guard, HEAD on GET, {$}, catch-all HTML", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "home", Method: "GET", Prefix: "/admin", Pattern: "/", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
			{Key: "orders", Method: "POST", Prefix: "/admin", Pattern: "/orders", Surface: handlers.HTML, Auth: handlers.Permission, Permission: "orders.write", Label: testRouteLabel},
		}
		g := &fakeGuard{}
		var events []string

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events = append(events, "mw1")
				next.ServeHTTP(w, r)
			})
		}
		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events = append(events, "mw2")
				next.ServeHTTP(w, r)
			})
		}

		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, g, handlers.WithMaxBody(8), handlers.WithMiddleware(mw1, mw2))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			rt, ok := handlers.From(r.Context())
			if !ok || rt.Key != "home" {
				t.Errorf("handler From() = (%+v, %v)", rt, ok)
			}
			_, _ = io.WriteString(w, "home")
			return nil
		})
		m.Handle(1, func(w http.ResponseWriter, r *http.Request) error {
			events = append(events, "handler")
			if _, err := io.ReadAll(r.Body); err != nil {
				return err
			}
			_, _ = io.WriteString(w, "ok")
			return nil
		})
		m.Done()

		// Prepare called once, only for non-public route.
		if len(g.prepared) != 1 || g.prepared[0].Key != "orders" {
			t.Fatalf("prepared = %+v, want [orders]", g.prepared)
		}

		// Public route skips guard.
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/", nil))
		if rec.Code != 200 || rec.Body.String() != "home" || len(g.checked) != 0 {
			t.Errorf("GET /admin/ = (%d, %q), checked=%d", rec.Code, rec.Body.String(), len(g.checked))
		}

		// HEAD on GET route works.
		recHead := httptest.NewRecorder()
		mux.ServeHTTP(recHead, httptest.NewRequest("HEAD", "/admin/", nil))
		if recHead.Code != 200 {
			t.Errorf("HEAD /admin/ = %d, want 200", recHead.Code)
		}

		// {$} does not match /admin/x -> catch-all 404 without Allow.
		recMiss := httptest.NewRecorder()
		mux.ServeHTTP(recMiss, httptest.NewRequest("GET", "/admin/x", nil))
		if recMiss.Code != 404 || recMiss.Body.String() != "not found" || recMiss.Header().Get("Allow") != "" {
			t.Errorf("GET /admin/x = (%d, %q, allow=%q)", recMiss.Code, recMiss.Body.String(), recMiss.Header().Get("Allow"))
		}

		// Known path with wrong method under prefix -> catch-all 404 without Allow.
		recWrongMethod := httptest.NewRecorder()
		mux.ServeHTTP(recWrongMethod, httptest.NewRequest("GET", "/admin/orders", nil))
		if recWrongMethod.Code != 404 || recWrongMethod.Body.String() != "not found" || recWrongMethod.Header().Get("Allow") != "" {
			t.Errorf("GET /admin/orders = (%d, %q, allow=%q)", recWrongMethod.Code, recWrongMethod.Body.String(), recWrongMethod.Header().Get("Allow"))
		}

		// Non-public route: route in context before guard, middleware order, body limit 413.
		events = nil
		recBig := httptest.NewRecorder()
		mux.ServeHTTP(recBig, httptest.NewRequest("POST", "/admin/orders", bytes.NewBufferString("123456789")))
		if recBig.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("POST big body = %d, want 413", recBig.Code)
		}
		if !g.sawRoute {
			t.Error("guard did not see route in context")
		}
		if strings.Join(events, ",") != "mw1,mw2,handler" {
			t.Errorf("middleware/handler order = %v, want [mw1 mw2 handler]", events)
		}

		// Guard refusal stops middleware and handler.
		events = nil
		g.checkErr = errTestGuardDeny
		recDeny := httptest.NewRecorder()
		mux.ServeHTTP(recDeny, httptest.NewRequest("POST", "/admin/orders", nil))
		if recDeny.Code != 403 || recDeny.Body.String() != "denied" || len(events) != 0 {
			t.Errorf("guard deny = (%d, %q, events=%v)", recDeny.Code, recDeny.Body.String(), events)
		}
	})

	t.Run("catch-all 404 in JSON format and custom WithErrors", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "api.ping", Method: "GET", Prefix: "/api", Pattern: "/ping", Surface: handlers.JSON, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil)
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error { return nil })
		m.Done()

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("POST", "/api/ping", nil))
		if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" || rec.Header().Get("Allow") != "" {
			t.Fatalf("POST /api/ping = (%d, ct=%q, allow=%q)", rec.Code, rec.Header().Get("Content-Type"), rec.Header().Get("Allow"))
		}
		var body struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != "errors.handlers.not_found" || body.Message != "not found" {
			t.Errorf("body = %+v", body)
		}
	})

	t.Run("no prefix registers no catch-all and leaves 405 with Allow", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "home", Method: "GET", Prefix: "", Pattern: "/", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
			{Key: "items", Method: "GET", Prefix: "", Pattern: "/items", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithMaxBody(0))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			_, _ = io.WriteString(w, "root")
			return nil
		})
		m.Handle(1, func(w http.ResponseWriter, r *http.Request) error {
			// WithMaxBody(0) allows >1 MiB body.
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(w, "%d", len(b))
			return nil
		})
		m.Done()

		rec405 := httptest.NewRecorder()
		mux.ServeHTTP(rec405, httptest.NewRequest("POST", "/items", nil))
		if rec405.Code != 405 || rec405.Header().Get("Allow") == "" {
			t.Errorf("POST /items = (%d, allow=%q), want 405 with Allow", rec405.Code, rec405.Header().Get("Allow"))
		}

		// WithMaxBody(0): 2 MiB body succeeds.
		big := bytes.Repeat([]byte("a"), 2<<20)
		recBig := httptest.NewRecorder()
		mux.ServeHTTP(recBig, httptest.NewRequest("GET", "/items", bytes.NewReader(big)))
		if recBig.Code != 200 || recBig.Body.String() != fmt.Sprintf("%d", 2<<20) {
			t.Errorf("GET /items 2MiB = (%d, %q)", recBig.Code, recBig.Body.String())
		}
	})

	t.Run("WithErrors overrides default writer", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "home", Method: "GET", Prefix: "/app", Pattern: "/", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
		}
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithErrors(func(w http.ResponseWriter, r *http.Request, err error) {
			w.WriteHeader(handlers.Status(err))
			_, _ = io.WriteString(w, "custom:"+errors.T(r.Context(), err))
		}))
		m.Handle(0, func(w http.ResponseWriter, r *http.Request) error {
			return handlers.ErrInvalidRequest
		})
		m.Done()

		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/app/", nil))
		if rec.Code != 400 || rec.Body.String() != "custom:the request is not valid" {
			t.Errorf("custom writer = (%d, %q)", rec.Code, rec.Body.String())
		}
	})

	t.Run("Route.MaxBody precedence over WithMaxBody", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "default", Method: "POST", Prefix: "/app", Pattern: "/default", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel},
			{Key: "larger", Method: "POST", Prefix: "/app", Pattern: "/larger", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel, MaxBody: 32},
			{Key: "smaller", Method: "POST", Prefix: "/app", Pattern: "/smaller", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel, MaxBody: 4},
		}
		readAllHandler := func(w http.ResponseWriter, r *http.Request) error {
			b, err := io.ReadAll(r.Body)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(w, "%d", len(b))
			return nil
		}

		// Registration limit = 8 bytes.
		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, nil, handlers.WithMaxBody(8))
		m.Handle(0, readAllHandler)
		m.Handle(1, readAllHandler)
		m.Handle(2, readAllHandler)
		m.Done()

		// 16-byte body: refused by default route (limit 8), admitted by larger route (MaxBody 32).
		body16 := strings.Repeat("x", 16)
		recDef := httptest.NewRecorder()
		mux.ServeHTTP(recDef, httptest.NewRequest("POST", "/app/default", strings.NewReader(body16)))
		if recDef.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("default route 16B = %d, want 413", recDef.Code)
		}

		recLarge := httptest.NewRecorder()
		mux.ServeHTTP(recLarge, httptest.NewRequest("POST", "/app/larger", strings.NewReader(body16)))
		if recLarge.Code != http.StatusOK || recLarge.Body.String() != "16" {
			t.Errorf("larger route 16B = (%d, %q), want (200, \"16\")", recLarge.Code, recLarge.Body.String())
		}

		// 6-byte body: admitted by default route (limit 8), refused by smaller route (MaxBody 4).
		body6 := strings.Repeat("x", 6)
		recDef6 := httptest.NewRecorder()
		mux.ServeHTTP(recDef6, httptest.NewRequest("POST", "/app/default", strings.NewReader(body6)))
		if recDef6.Code != http.StatusOK || recDef6.Body.String() != "6" {
			t.Errorf("default route 6B = (%d, %q), want (200, \"6\")", recDef6.Code, recDef6.Body.String())
		}

		recSmall6 := httptest.NewRecorder()
		mux.ServeHTTP(recSmall6, httptest.NewRequest("POST", "/app/smaller", strings.NewReader(body6)))
		if recSmall6.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("smaller route 6B = %d, want 413", recSmall6.Code)
		}

		// WithMaxBody(0) still applies a route's own MaxBody.
		muxZero := http.NewServeMux()
		mZero := handlers.NewMount(muxZero, routes, nil, handlers.WithMaxBody(0))
		mZero.Handle(0, readAllHandler)
		mZero.Handle(1, readAllHandler)
		mZero.Handle(2, readAllHandler)
		mZero.Done()

		recZeroSmall := httptest.NewRecorder()
		muxZero.ServeHTTP(recZeroSmall, httptest.NewRequest("POST", "/app/smaller", strings.NewReader(body6)))
		if recZeroSmall.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("WithMaxBody(0) with route MaxBody=4 on 6B = %d, want 413", recZeroSmall.Code)
		}
	})
}

func TestMountHandleHTTPAndWriter(t *testing.T) {
	t.Run("HandleHTTP panics on nil handler and missing Done", func(t *testing.T) {
		pub := handlers.Route{Key: "api.ping", Method: "GET", Prefix: "/api", Pattern: "/ping", Surface: handlers.JSON, Auth: handlers.Public, Label: testRouteLabel}
		assertPanics(t, "nil http.Handler", func() {
			m := handlers.NewMount(http.NewServeMux(), []handlers.Route{pub}, nil)
			m.HandleHTTP(0, nil, nil)
		})
		assertPanics(t, "duplicate HandleHTTP", func() {
			m := handlers.NewMount(http.NewServeMux(), []handlers.Route{pub}, nil)
			m.HandleHTTP(0, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil)
			m.HandleHTTP(0, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}), nil)
		})
	})

	t.Run("HandleHTTP chain order: route, guard, middleware, validate, handler", func(t *testing.T) {
		routes := []handlers.Route{
			{Key: "api.orders.create", Method: "POST", Prefix: "/api", Pattern: "/orders", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "orders.write", Label: testRouteLabel},
			{Key: "api.ping", Method: "GET", Prefix: "/api", Pattern: "/ping", Surface: handlers.JSON, Auth: handlers.Public, Label: testRouteLabel},
		}
		g := &fakeGuard{}
		var events []string
		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				events = append(events, "mw")
				next.ServeHTTP(w, r)
			})
		}

		mux := http.NewServeMux()
		m := handlers.NewMount(mux, routes, g, handlers.WithMiddleware(mw))

		var validateErr error
		m.HandleHTTP(0, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			events = append(events, "handler")
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, "created")
		}), func(r *http.Request) error {
			events = append(events, "validate")
			if _, ok := handlers.From(r.Context()); !ok {
				t.Error("validate did not see route in context")
			}
			return validateErr
		})
		m.HandleHTTP(1, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "pong")
		}), nil)
		m.Done()

		// Success with nil validate.
		recPing := httptest.NewRecorder()
		mux.ServeHTTP(recPing, httptest.NewRequest("GET", "/api/ping", nil))
		if recPing.Code != 200 || recPing.Body.String() != "pong" {
			t.Errorf("GET /api/ping = (%d, %q)", recPing.Code, recPing.Body.String())
		}

		// Guard refusal skips middleware, validate, handler.
		events = nil
		g.checkErr = errTestGuardDeny
		recDeny := httptest.NewRecorder()
		mux.ServeHTTP(recDeny, httptest.NewRequest("POST", "/api/orders", nil))
		if recDeny.Code != 403 || len(events) != 0 {
			t.Errorf("guard deny = (%d, events=%v)", recDeny.Code, events)
		}

		// Validation error runs after middleware, stops handler, writes JSON.
		g.checkErr = nil
		validateErr = handlers.ErrInvalidField.With("/items")
		events = nil
		recBad := httptest.NewRecorder()
		mux.ServeHTTP(recBad, httptest.NewRequest("POST", "/api/orders", nil))
		if recBad.Code != 400 || recBad.Header().Get("Content-Type") != "application/json" {
			t.Errorf("validate err = (%d, ct=%q)", recBad.Code, recBad.Header().Get("Content-Type"))
		}
		if strings.Join(events, ",") != "mw,validate" {
			t.Errorf("events = %v, want [mw validate]", events)
		}

		// Validation passes -> handler runs.
		validateErr = nil
		events = nil
		recOK := httptest.NewRecorder()
		mux.ServeHTTP(recOK, httptest.NewRequest("POST", "/api/orders", nil))
		if recOK.Code != 201 || recOK.Body.String() != "created" {
			t.Errorf("POST /api/orders = (%d, %q)", recOK.Code, recOK.Body.String())
		}
		if strings.Join(events, ",") != "mw,validate,handler" {
			t.Errorf("events = %v, want [mw validate handler]", events)
		}
	})

	t.Run("Writer returns WithErrors option or surface default", func(t *testing.T) {
		mux := http.NewServeMux()
		jsonRoutes := []handlers.Route{{Key: "api.x", Method: "GET", Pattern: "/x", Surface: handlers.JSON, Auth: handlers.Public, Label: testRouteLabel}}
		htmlRoutes := []handlers.Route{{Key: "ui.x", Method: "GET", Pattern: "/x", Surface: handlers.HTML, Auth: handlers.Public, Label: testRouteLabel}}

		mJSON := handlers.NewMount(mux, jsonRoutes, nil)
		rec := httptest.NewRecorder()
		mJSON.Writer()(rec, httptest.NewRequest("GET", "/x", nil), handlers.ErrNotFound)
		if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("json default writer = (%d, %q)", rec.Code, rec.Header().Get("Content-Type"))
		}

		mHTML := handlers.NewMount(mux, htmlRoutes, nil)
		rec2 := httptest.NewRecorder()
		mHTML.Writer()(rec2, httptest.NewRequest("GET", "/x", nil), handlers.ErrNotFound)
		if rec2.Code != 404 || rec2.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("html default writer = (%d, %q)", rec2.Code, rec2.Header().Get("Content-Type"))
		}

		called := false
		custom := func(w http.ResponseWriter, r *http.Request, err error) {
			called = true
			w.WriteHeader(http.StatusTeapot)
		}
		mCustom := handlers.NewMount(mux, nil, nil, handlers.WithErrors(custom))
		rec3 := httptest.NewRecorder()
		mCustom.Writer()(rec3, httptest.NewRequest("GET", "/", nil), handlers.ErrNotFound)
		if !called || rec3.Code != http.StatusTeapot {
			t.Errorf("custom writer = (called=%v, code=%d)", called, rec3.Code)
		}

		mEmpty := handlers.NewMount(mux, nil, nil)
		if mEmpty.Writer() != nil {
			t.Errorf("empty mount default writer = non-nil, want nil")
		}
	})
}

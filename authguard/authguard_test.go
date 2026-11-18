package authguard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/epistemer/auth"
	"github.com/epistemer/auth/access"
	"github.com/epistemer/auth/authtest"
	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/authguard"
	"github.com/epistemer/i18n"
	bolt "go.etcd.io/bbolt"
)

var (
	permOrdersManage = access.Permission("orders.manage", "Manage orders")
	permOrdersCancel = access.Permission("orders.cancel", "Cancel orders")
	permReportsView  = access.Permission("reports.view", "View reports", auth.Unverified)
)

const testPassword = "orange tiger juggles 47 lamps"

type testEnv struct {
	t   *testing.T
	kit *authtest.Kit
	db  *bolt.DB
	ac  *access.Access
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	k := authtest.New(t, auth.WithPolicy(func(context.Context, auth.User) auth.Policy {
		return auth.Policy{AllowUnverified: true}
	}))
	db, err := bolt.Open(filepath.Join(t.TempDir(), "access.db"), 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ac, err := access.New(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	k.AddUser("sara", auth.User{ID: "u1", Name: "sara"}, "sara@toyshop.example")
	if err := k.Auth.SetPassword(ctx, "u1", testPassword); err != nil {
		t.Fatal(err)
	}
	k.AddUser("omar", auth.User{ID: "u2", Name: "omar"}, "omar@toyshop.example")
	if err := k.Auth.SetPassword(ctx, "u2", testPassword); err != nil {
		t.Fatal(err)
	}

	return &testEnv{t: t, kit: k, db: db, ac: ac}
}

func (e *testEnv) signIn(login string, verified bool) *http.Cookie {
	e.t.Helper()
	ctx := context.Background()
	a := e.kit.Auth
	f, err := a.SignIn(ctx, login, auth.Cookie())
	if err != nil {
		e.t.Fatal(err)
	}
	if f, err = a.Prove(ctx, f.Token, auth.Password(testPassword)); err != nil {
		e.t.Fatal(err)
	}
	if verified {
		e.kit.Clock.Advance(time.Minute)
		if _, err := a.Challenge(ctx, f.Token, auth.MethodEmailCode); err != nil {
			e.t.Fatal(err)
		}
		m, ok := e.kit.LastMessage(login + "@toyshop.example")
		if !ok {
			e.t.Fatalf("no email code sent to %s", login)
		}
		if f, err = a.Prove(ctx, f.Token, auth.EmailCode(m.Code)); err != nil || !f.Verified {
			e.t.Fatalf("second proof: %+v %v", f, err)
		}
	}
	res, err := a.Finish(ctx, f.Token)
	if err != nil {
		e.t.Fatal(err)
	}
	w := httptest.NewRecorder()
	res.Write(w)
	for _, c := range w.Result().Cookies() {
		if c.Name == "__Host-session" {
			return c
		}
	}
	e.t.Fatal("no session cookie issued")
	return nil
}

func assertPanics(t *testing.T, name string, fn func()) string {
	t.Helper()
	var msg string
	func() {
		defer func() {
			if r := recover(); r != nil {
				msg = fmt.Sprint(r)
			}
		}()
		fn()
	}()
	if msg == "" {
		t.Fatalf("%s: expected panic, got none", name)
	}
	return msg
}

type logCapture struct {
	mu      sync.Mutex
	records []string
}

func (l *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (l *logCapture) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.records = append(l.records, fmt.Sprintf("%s %s", r.Level, r.Message))
	return nil
}
func (l *logCapture) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logCapture) WithGroup(string) slog.Handler      { return l }

func testRoutes() []handlers.Route {
	lbl := i18n.Key("test.route", "Route")
	return []handlers.Route{
		{Key: "home", Method: "GET", Prefix: "/admin", Pattern: "/", Surface: handlers.JSON, Auth: handlers.Public, Label: lbl},
		{Key: "profile", Method: "GET", Prefix: "/admin", Pattern: "/profile", Surface: handlers.JSON, Auth: handlers.Session, Label: lbl},
		{Key: "orders.list", Method: "GET", Prefix: "/admin", Pattern: "/orders", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "orders.manage", Label: lbl},
		{Key: "orders.cancel", Method: "POST", Prefix: "/admin", Pattern: "/orders/cancel", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "orders.manage", Label: lbl},
		{Key: "orders.cancel_perm", Method: "POST", Prefix: "/admin", Pattern: "/orders/cancel2", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "orders.cancel", Label: lbl},
		{Key: "reports.view", Method: "GET", Prefix: "/admin", Pattern: "/reports", Surface: handlers.JSON, Auth: handlers.Permission, Permission: "reports.view", Label: lbl},
	}
}

func TestNewPanicsOnNil(t *testing.T) {
	env := newTestEnv(t)
	assertPanics(t, "nil auth", func() { authguard.New(nil, env.ac) })
	assertPanics(t, "nil access", func() { authguard.New(env.kit.Auth, nil) })
}

func TestPrepareAndMount(t *testing.T) {
	env := newTestEnv(t)
	g := authguard.New(env.kit.Auth, env.ac)

	// Undeclared permission makes Prepare fail and Mount panic with the required message.
	undeclared := handlers.Route{
		Key:        "secret.route",
		Method:     "GET",
		Prefix:     "/admin",
		Pattern:    "/secret",
		Surface:    handlers.JSON,
		Auth:       handlers.Permission,
		Permission: "secret.missing",
		Label:      i18n.Key("test.secret", "Secret"),
	}
	err := g.Prepare(undeclared)
	wantMsg := `authguard: route "secret.route": permission "secret.missing" isn't declared`
	if err == nil || err.Error() != wantMsg {
		t.Fatalf("Prepare(undeclared) = %v, want %q", err, wantMsg)
	}

	panicMsg := assertPanics(t, "Mount with undeclared permission", func() {
		m := handlers.NewMount(http.NewServeMux(), []handlers.Route{undeclared}, g)
		m.Handle(0, func(http.ResponseWriter, *http.Request) error { return nil })
	})
	if !strings.Contains(panicMsg, wantMsg) {
		t.Errorf("Mount panic = %q, want substring %q", panicMsg, wantMsg)
	}

	// Unknown Auth panics per interface §12.2.
	bad := handlers.Route{Key: "bad", Auth: handlers.Auth(99)}
	assertPanics(t, "Prepare unknown auth", func() { _ = g.Prepare(bad) })
	assertPanics(t, "Check unknown auth", func() { _ = g.Check(httptest.NewRequest("GET", "/", nil), bad) })
}

func TestMountAndCheckWithAuthtest(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	grp, err := env.ac.CreateGroup(ctx, "Managers")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.ac.Grant(ctx, grp.ID, permOrdersManage); err != nil {
		t.Fatal(err)
	}
	if err := env.ac.Grant(ctx, grp.ID, permReportsView); err != nil {
		t.Fatal(err)
	}
	// sara ("u1") is in Managers; omar ("u2") is in no group.
	if err := env.ac.Join(ctx, "u1", grp.ID); err != nil {
		t.Fatal(err)
	}

	routes := testRoutes()
	g := authguard.New(env.kit.Auth, env.ac)
	mux := http.NewServeMux()
	m := handlers.NewMount(mux, routes, g, handlers.WithErrors(handlers.WriteJSON))
	for i := range routes {
		rt := routes[i]
		m.Handle(i, func(w http.ResponseWriter, r *http.Request) error {
			_, _ = io.WriteString(w, "ok:"+rt.Key)
			return nil
		})
	}
	m.Done()

	handler := env.kit.Auth.Load(auth.Cookie())(mux)

	doReq := func(method, path string, c *http.Cookie) (int, string, string) {
		t.Helper()
		req := httptest.NewRequest(method, "https://shop.example"+path, nil)
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code >= 400 {
			var body struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("unmarshal error body %q: %v", rec.Body.String(), err)
			}
			return rec.Code, body.Code, rec.Body.String()
		}
		return rec.Code, "", rec.Body.String()
	}

	saraVerified := env.signIn("sara", true)
	saraUnverified := env.signIn("sara", false)
	omarVerified := env.signIn("omar", true)

	// Public route admits without session.
	if code, _, body := doReq("GET", "/admin/", nil); code != 200 || body != "ok:home" {
		t.Errorf("public without session = (%d, %q)", code, body)
	}

	// Session route: 401 without session, 403 for unverified, 200 for verified (even without grants).
	if code, errCode, _ := doReq("GET", "/admin/profile", nil); code != 401 || errCode != "errors.auth.no_session" {
		t.Errorf("session without cookie = (%d, %q), want (401, errors.auth.no_session)", code, errCode)
	}
	if code, errCode, _ := doReq("GET", "/admin/profile", saraUnverified); code != 403 || errCode != "errors.auth.not_verified" {
		t.Errorf("session unverified = (%d, %q), want (403, errors.auth.not_verified)", code, errCode)
	}
	if code, _, body := doReq("GET", "/admin/profile", omarVerified); code != 200 || body != "ok:profile" {
		t.Errorf("session verified = (%d, %q)", code, body)
	}

	// Default (Verified) permission route: 401 no session, 403 unverified, 403 without grant, 200 with grant.
	if code, errCode, _ := doReq("GET", "/admin/orders", nil); code != 401 || errCode != "errors.auth.no_session" {
		t.Errorf("perm without session = (%d, %q)", code, errCode)
	}
	if code, errCode, _ := doReq("GET", "/admin/orders", saraUnverified); code != 403 || errCode != "errors.auth.not_verified" {
		t.Errorf("perm unverified = (%d, %q)", code, errCode)
	}
	if code, errCode, _ := doReq("GET", "/admin/orders", omarVerified); code != 403 || errCode != "errors.auth.forbidden" {
		t.Errorf("perm without grant = (%d, %q)", code, errCode)
	}
	if code, _, body := doReq("GET", "/admin/orders", saraVerified); code != 200 || body != "ok:orders.list" {
		t.Errorf("perm with grant = (%d, %q)", code, body)
	}

	// Unverified permission route: admits unverified session with grant, refuses without grant, refuses without session.
	if code, errCode, _ := doReq("GET", "/admin/reports", nil); code != 401 || errCode != "errors.auth.no_session" {
		t.Errorf("unverified perm without session = (%d, %q)", code, errCode)
	}
	if code, errCode, _ := doReq("GET", "/admin/reports", omarVerified); code != 403 || errCode != "errors.auth.forbidden" {
		t.Errorf("unverified perm without grant = (%d, %q)", code, errCode)
	}
	if code, _, body := doReq("GET", "/admin/reports", saraUnverified); code != 200 || body != "ok:reports.view" {
		t.Errorf("unverified perm with unverified session = (%d, %q)", code, body)
	}

	// Direct Check fails closed on Public route, nil request, and undeclared permission.
	if err := g.Check(httptest.NewRequest("GET", "/", nil), routes[0]); err == nil {
		t.Error("Check(public) = nil, want error")
	}
	if err := g.Check(nil, routes[1]); err == nil {
		t.Error("Check(nil req, session) = nil, want error")
	}
	if err := g.Check(nil, routes[2]); err == nil {
		t.Error("Check(nil req, perm) = nil, want error")
	}
	if err := g.Check(httptest.NewRequest("GET", "/", nil), handlers.Route{Key: "x", Auth: handlers.Permission, Permission: "nope"}); err == nil {
		t.Error("Check(undeclared perm) = nil, want error")
	}
}

func TestAdmitResultsMemoizationAndErrors(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	grp, err := env.ac.CreateGroup(ctx, "Staff")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.ac.Grant(ctx, grp.ID, permOrdersManage); err != nil {
		t.Fatal(err)
	}
	if err := env.ac.Grant(ctx, grp.ID, permReportsView); err != nil {
		t.Fatal(err)
	}
	if err := env.ac.Join(ctx, "u1", grp.ID); err != nil {
		t.Fatal(err)
	}

	routes := testRoutes()
	pub := routes[0]
	sess := routes[1]
	ordersList := routes[2]
	ordersCancel := routes[3]
	ordersCancelPerm := routes[4]
	reportsView := routes[5]
	undeclared := handlers.Route{Key: "x", Auth: handlers.Permission, Permission: "nope"}

	g := authguard.New(env.kit.Auth, env.ac)

	// Helper that runs Load(Cookie()) to attach the request's session to req.Context().
	loadReq := func(c *http.Cookie) *http.Request {
		t.Helper()
		var loaded *http.Request
		h := env.kit.Auth.Load(auth.Cookie())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			loaded = r
		}))
		req := httptest.NewRequest("GET", "https://shop.example/", nil)
		if c != nil {
			req.AddCookie(c)
		}
		h.ServeHTTP(httptest.NewRecorder(), req)
		return loaded
	}

	saraVerified := env.signIn("sara", true)
	saraUnverified := env.signIn("sara", false)
	omarVerified := env.signIn("omar", true)

	t.Run("no session", func(t *testing.T) {
		admit, err := g.Admit(loadReq(nil))
		if err != nil {
			t.Fatalf("Admit error = %v", err)
		}
		if !admit(pub) || admit(sess) || admit(ordersList) || admit(reportsView) || admit(undeclared) {
			t.Errorf("unexpected Admit results for anonymous request")
		}
	})

	t.Run("nil request", func(t *testing.T) {
		admit, err := g.Admit(nil)
		if err != nil {
			t.Fatalf("Admit(nil) error = %v", err)
		}
		if !admit(pub) || admit(sess) || admit(ordersList) {
			t.Errorf("unexpected Admit(nil) results")
		}
	})

	t.Run("unverified session", func(t *testing.T) {
		admit, err := g.Admit(loadReq(saraUnverified))
		if err != nil {
			t.Fatalf("Admit error = %v", err)
		}
		if !admit(pub) || admit(sess) || admit(ordersList) || !admit(reportsView) || admit(undeclared) {
			t.Errorf("unexpected Admit results for unverified session: pub=%v sess=%v orders=%v reports=%v",
				admit(pub), admit(sess), admit(ordersList), admit(reportsView))
		}
	})

	t.Run("verified session without grant", func(t *testing.T) {
		admit, err := g.Admit(loadReq(omarVerified))
		if err != nil {
			t.Fatalf("Admit error = %v", err)
		}
		if !admit(pub) || !admit(sess) || admit(ordersList) || admit(reportsView) {
			t.Errorf("unexpected Admit results for verified session without grants")
		}
	})

	t.Run("verified session with grant and per-permission memoization", func(t *testing.T) {
		req := loadReq(saraVerified)
		admit, err := g.Admit(req)
		if err != nil {
			t.Fatalf("Admit error = %v", err)
		}
		if !admit(ordersList) {
			t.Fatal("expected ordersList admitted before withdraw")
		}

		// Withdraw orders.manage and grant orders.cancel while keeping the same admit func.
		if err := env.ac.Withdraw(ctx, grp.ID, permOrdersManage); err != nil {
			t.Fatal(err)
		}
		if err := env.ac.Grant(ctx, grp.ID, permOrdersCancel); err != nil {
			t.Fatal(err)
		}

		// ordersCancel shares orders.manage, which was already checked -> memoized true.
		if !admit(ordersCancel) {
			t.Error("ordersCancel should use memoized result for orders.manage")
		}
		// ordersCancelPerm has a distinct permission not yet queried -> reads DB lazily -> true.
		if !admit(ordersCancelPerm) {
			t.Error("ordersCancelPerm should be checked lazily and succeed")
		}

		// A fresh Admit sees the withdrawn orders.manage as false.
		admit2, err := g.Admit(req)
		if err != nil {
			t.Fatal(err)
		}
		if admit2(ordersList) || admit2(ordersCancel) {
			t.Error("fresh Admit should refuse withdrawn orders.manage")
		}
		if !admit2(ordersCancelPerm) {
			t.Error("fresh Admit should admit granted orders.cancel")
		}

		// Restore grant for subsequent subtests.
		if err := env.ac.Grant(ctx, grp.ID, permOrdersManage); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("concurrent Admit calls under race detector", func(t *testing.T) {
		admit, err := g.Admit(loadReq(saraVerified))
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range 20 {
					if !admit(pub) || !admit(sess) || !admit(ordersList) || !admit(ordersCancel) || !admit(reportsView) {
						t.Error("expected all routes admitted concurrently")
					}
					if admit(undeclared) {
						t.Error("expected undeclared refused")
					}
				}
			}()
		}
		wg.Wait()
	})

	t.Run("unknown auth panics in Admit per §12.2", func(t *testing.T) {
		admit, err := g.Admit(loadReq(saraVerified))
		if err != nil {
			t.Fatal(err)
		}
		assertPanics(t, "Admit unknown auth", func() {
			_ = admit(handlers.Route{Key: "bad", Auth: handlers.Auth(99)})
		})
	})

	t.Run("non-refusal error is logged once at warn level and returns false", func(t *testing.T) {
		orig := slog.Default()
		cap := &logCapture{}
		slog.SetDefault(slog.New(cap))
		t.Cleanup(func() { slog.SetDefault(orig) })

		req := loadReq(saraVerified)
		admit, err := g.Admit(req)
		if err != nil {
			t.Fatal(err)
		}
		// Close access DB so ac.Check returns bolt.ErrDatabaseNotOpen.
		if err := env.db.Close(); err != nil {
			t.Fatal(err)
		}
		if admit(ordersList) {
			t.Error("expected false when access DB is closed")
		}
		if admit(ordersCancel) || admit(ordersCancelPerm) {
			t.Error("expected false on repeated/other permission checks with closed DB")
		}
		if len(cap.records) != 1 || !strings.HasPrefix(cap.records[0], "WARN ") {
			t.Errorf("expected exactly 1 WARN log record, got %v", cap.records)
		}
	})

	t.Run("normal refusals are not logged", func(t *testing.T) {
		env2 := newTestEnv(t)
		g2 := authguard.New(env2.kit.Auth, env2.ac)
		omar := env2.signIn("omar", true)

		var loaded *http.Request
		env2.kit.Auth.Load(auth.Cookie())(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			loaded = r
		})).ServeHTTP(httptest.NewRecorder(), func() *http.Request {
			req := httptest.NewRequest("GET", "https://shop.example/", nil)
			req.AddCookie(omar)
			return req
		}())

		orig := slog.Default()
		cap := &logCapture{}
		slog.SetDefault(slog.New(cap))
		t.Cleanup(func() { slog.SetDefault(orig) })

		admit, err := g2.Admit(loaded)
		if err != nil {
			t.Fatal(err)
		}
		if admit(ordersList) {
			t.Error("expected false for ungranted permission")
		}
		if len(cap.records) != 0 {
			t.Errorf("unexpected warn logs on normal ErrForbidden: %v", cap.records)
		}
	})
}

func TestErrForbiddenIsKnown(t *testing.T) {
	// Sanity check that auth's refusal errors map to 401/403 via errors.KindOf.
	if got := handlers.Status(auth.ErrNoSession); got != http.StatusUnauthorized {
		t.Errorf("Status(auth.ErrNoSession) = %d, want 401", got)
	}
	for _, err := range []error{auth.ErrNotVerified, auth.ErrStale, auth.ErrForbidden} {
		if got := handlers.Status(err); got != http.StatusForbidden {
			t.Errorf("Status(%v) = %d, want 403", err, got)
		}
		if !errors.Is(err, err) {
			t.Errorf("errors.Is(%v) = false", err)
		}
	}
}

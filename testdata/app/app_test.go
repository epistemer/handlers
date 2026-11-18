package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"app/admin"
	"app/adminroutes"
	"app/ids"
	"github.com/epistemer/auth"
	"github.com/epistemer/auth/access"
	"github.com/epistemer/auth/authtest"
	"github.com/epistemer/genx/genxtest"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/authguard"
	"github.com/epistemer/handlers/handlerstest"
	"github.com/epistemer/i18n"
	"github.com/epistemer/i18n/i18ntest"
	bolt "go.etcd.io/bbolt"
)

func TestGenerated(t *testing.T) {
	genxtest.AssertGenerated(t)
}

func TestPublicRoutes(t *testing.T) {
	handlerstest.AssertPublic(t, adminroutes.Routes, "dashboard", "files.serve")
}

func TestAdminRoutesHTTPAndLabels(t *testing.T) {
	tr := i18ntest.New(t, i18n.Locale{Code: "en", NativeName: "English"})
	if err := tr.CreateLocale(i18n.Locale{Code: "fr"}); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route.orders.detail", "Détail de commande"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Set("fr", "route_param.orders.detail.id", "ID commande"); err != nil {
		t.Fatal(err)
	}

	h := &admin.H{}
	g := &handlerstest.Guard{
		Session: true,
		Granted: map[string]bool{"orders.manage": true},
	}
	mux := http.NewServeMux()
	adminroutes.Register(mux, h, g)

	t.Run("nil guard panics when non-public routes exist", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic with nil guard")
			}
		}()
		adminroutes.Register(http.NewServeMux(), &admin.H{}, nil)
	})

	t.Run("nil handler panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic with nil handler")
			}
		}()
		adminroutes.Register(http.NewServeMux(), nil, g)
	})

	t.Run("root route answers /admin/ and not /admin/x", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/", nil))
		if rec.Code != 200 || rec.Body.String() != "dashboard" {
			t.Errorf("GET /admin/ = (%d, %q)", rec.Code, rec.Body.String())
		}

		recX := httptest.NewRecorder()
		mux.ServeHTTP(recX, httptest.NewRequest("GET", "/admin/x", nil))
		if recX.Code != 404 || recX.Body.String() != "not found" {
			t.Errorf("GET /admin/x = (%d, %q)", recX.Code, recX.Body.String())
		}
	})

	t.Run("unknown path under /admin returns 404 text", func(t *testing.T) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/unknown/path", nil))
		if rec.Code != 404 || rec.Body.String() != "not found" {
			t.Errorf("GET /admin/unknown/path = (%d, %q)", rec.Code, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
			t.Errorf("Content-Type = %q", ct)
		}
	})

	t.Run("bad int param returns 400 with ErrBadParam English text", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/admin/orders/notanint/tok1", nil).WithContext(i18ntest.Context(t, tr, "en"))
		mux.ServeHTTP(rec, req)
		if rec.Code != 400 || rec.Body.String() != "Order ID is not valid" {
			t.Errorf("bad int = (%d, %q), want (400, %q)", rec.Code, rec.Body.String(), "Order ID is not valid")
		}

		// Plain context (no i18n translator) also renders English default.
		recPlain := httptest.NewRecorder()
		mux.ServeHTTP(recPlain, httptest.NewRequest("GET", "/admin/orders/notanint/tok1", nil))
		if recPlain.Code != 400 || recPlain.Body.String() != "Order ID is not valid" {
			t.Errorf("bad int plain = (%d, %q)", recPlain.Code, recPlain.Body.String())
		}

		// Translated locale renders translated param label inside ErrBadParam.
		recFR := httptest.NewRecorder()
		reqFR := httptest.NewRequest("GET", "/admin/orders/notanint/tok1", nil).WithContext(i18ntest.Context(t, tr, "fr"))
		mux.ServeHTTP(recFR, reqFR)
		if recFR.Code != 400 || recFR.Body.String() != "ID commande is not valid" {
			t.Errorf("bad int fr = (%d, %q)", recFR.Code, recFR.Body.String())
		}
	})

	t.Run("bad text and uint16 overflow return 400", func(t *testing.T) {
		recText := httptest.NewRecorder()
		mux.ServeHTTP(recText, httptest.NewRequest("GET", "/admin/orders/42/bad", nil))
		if recText.Code != 400 || recText.Body.String() != "Access token is not valid" {
			t.Errorf("bad text = (%d, %q)", recText.Code, recText.Body.String())
		}

		recUint := httptest.NewRecorder()
		mux.ServeHTTP(recUint, httptest.NewRequest("POST", "/admin/flags/true/70000", nil))
		if recUint.Code != 400 || recUint.Body.String() != "Count is not valid" {
			t.Errorf("uint16 overflow = (%d, %q)", recUint.Code, recUint.Body.String())
		}
	})

	t.Run("handler sees Route via handlers.From and handlerstest.Guard enforces auth", func(t *testing.T) {
		h.LastRoute = handlers.Route{}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/admin/orders/42/tok1", nil))
		if rec.Code != 200 || rec.Body.String() != "order:42:tok1" {
			t.Fatalf("GET /admin/orders/42/tok1 = (%d, %q)", rec.Code, rec.Body.String())
		}
		if h.LastRoute.Key != "orders.detail" || h.LastRoute.Permission != "orders.manage" || h.LastRoute.Entity != "orders.list" {
			t.Errorf("handler route = %+v", h.LastRoute)
		}

		// Denied without permission -> 403; denied without session -> 401.
		g.Granted = nil
		rec403 := httptest.NewRecorder()
		mux.ServeHTTP(rec403, httptest.NewRequest("GET", "/admin/orders/42/tok1", nil))
		if rec403.Code != 403 {
			t.Errorf("without permission = %d, want 403", rec403.Code)
		}
		g.Session = false
		rec401 := httptest.NewRecorder()
		mux.ServeHTTP(rec401, httptest.NewRequest("GET", "/admin/orders", nil))
		if rec401.Code != 401 {
			t.Errorf("without session = %d, want 401", rec401.Code)
		}
		g.Session = true
		g.Granted = map[string]bool{"orders.manage": true}
	})

	t.Run("URL builders escape segments and preserve rest slashes", func(t *testing.T) {
		if got := adminroutes.Dashboard(); got != "/admin/" {
			t.Errorf("Dashboard() = %q", got)
		}
		if got := adminroutes.Orders(); got != "/admin/orders" {
			t.Errorf("Orders() = %q", got)
		}
		if got := adminroutes.Order(42, ids.Token{Raw: "a/b c"}); got != "/admin/orders/42/a%2Fb%20c" {
			t.Errorf("Order() = %q", got)
		}
		if got := adminroutes.OrderCancel(42); got != "/admin/orders/42/cancel" {
			t.Errorf("OrderCancel() = %q", got)
		}
		if got := adminroutes.Files("dir/sub dir/file?1.txt"); got != "/admin/files/dir/sub%20dir/file%3F1.txt" {
			t.Errorf("Files() = %q", got)
		}
		if got := adminroutes.Flags(true, 9); got != "/admin/flags/true/9" {
			t.Errorf("Flags() = %q", got)
		}
	})

	t.Run("ByKey and i18ntest label translations", func(t *testing.T) {
		rt, ok := adminroutes.ByKey("orders.detail")
		if !ok || rt.Key != "orders.detail" || rt.Entity != "orders.list" || len(rt.Params) != 2 {
			t.Fatalf("ByKey(orders.detail) = (%+v, %v)", rt, ok)
		}
		if _, ok := adminroutes.ByKey("missing"); ok {
			t.Error("ByKey(missing) = true, want false")
		}

		ctxEN := i18ntest.Context(t, tr, "en")
		ctxFR := i18ntest.Context(t, tr, "fr")

		if got := rt.Label.T(ctxEN); got != "Order detail" {
			t.Errorf("route label en = %q", got)
		}
		if got := rt.Label.T(ctxFR); got != "Détail de commande" {
			t.Errorf("route label fr = %q", got)
		}
		if got := rt.Params[0].Label.T(ctxFR); got != "ID commande" {
			t.Errorf("param label fr = %q", got)
		}
		if got := adminroutes.L10n("route.orders.detail").T(ctxFR); got != "Détail de commande" {
			t.Errorf("L10n lookup fr = %q", got)
		}
	})
}

func TestAdminRoutesWithAuthguard(t *testing.T) {
	k := authtest.New(t)
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
	const pw = "orange tiger juggles 47 lamps"
	k.AddUser("sara", auth.User{ID: "u1", Name: "sara"}, "sara@toyshop.example")
	if err := k.Auth.SetPassword(ctx, "u1", pw); err != nil {
		t.Fatal(err)
	}
	grp, err := ac.CreateGroup(ctx, "Managers")
	if err != nil {
		t.Fatal(err)
	}
	if err := ac.Grant(ctx, grp.ID, adminroutes.PermOrdersManage); err != nil {
		t.Fatal(err)
	}
	if err := ac.Join(ctx, "u1", grp.ID); err != nil {
		t.Fatal(err)
	}

	f, err := k.Auth.SignIn(ctx, "sara", auth.Cookie())
	if err != nil {
		t.Fatal(err)
	}
	if f, err = k.Auth.Prove(ctx, f.Token, auth.Password(pw)); err != nil {
		t.Fatal(err)
	}
	k.Clock.Advance(time.Minute)
	if _, err := k.Auth.Challenge(ctx, f.Token, auth.MethodEmailCode); err != nil {
		t.Fatal(err)
	}
	m, _ := k.LastMessage("sara@toyshop.example")
	if f, err = k.Auth.Prove(ctx, f.Token, auth.EmailCode(m.Code)); err != nil {
		t.Fatal(err)
	}
	res, err := k.Auth.Finish(ctx, f.Token)
	if err != nil {
		t.Fatal(err)
	}
	cw := httptest.NewRecorder()
	res.Write(cw)
	cookie := cw.Result().Cookies()[0]

	mux := http.NewServeMux()
	adminroutes.Register(mux, &admin.H{}, authguard.New(k.Auth, ac))
	srv := k.Auth.Load(auth.Cookie())(mux)

	// Unauthenticated -> 401; authenticated with generated PermOrdersManage -> 200.
	recAnon := httptest.NewRecorder()
	srv.ServeHTTP(recAnon, httptest.NewRequest("GET", "https://shop.example/admin/orders/7/tok", nil))
	if recAnon.Code != 401 {
		t.Errorf("anon = %d, want 401", recAnon.Code)
	}

	req := httptest.NewRequest("GET", "https://shop.example/admin/orders/7/tok", nil)
	req.AddCookie(cookie)
	recAuth := httptest.NewRecorder()
	srv.ServeHTTP(recAuth, req)
	if recAuth.Code != 200 || recAuth.Body.String() != "order:7:tok" {
		t.Errorf("authed = (%d, %q), want (200, order:7:tok)", recAuth.Code, recAuth.Body.String())
	}
}

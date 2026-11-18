package handlerstest_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
	"github.com/epistemer/handlers/handlerstest"
)

var (
	_ handlers.Guard = handlerstest.Guard{}
	_ handlers.Guard = (*handlerstest.Guard)(nil)
)

type recordingTB struct {
	testing.TB
	errs []string
}

func (r *recordingTB) Helper() {}

func (r *recordingTB) Errorf(format string, args ...any) {
	r.errs = append(r.errs, fmt.Sprintf(format, args...))
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

func TestGuardErrorsAndStatuses(t *testing.T) {
	if got := errors.Public(handlerstest.ErrNoSession).K(); got != "errors.handlerstest.no_session" {
		t.Errorf("ErrNoSession key = %q", got)
	}
	if got := errors.KindOf(handlerstest.ErrNoSession); got != errors.Unauthenticated {
		t.Errorf("ErrNoSession kind = %v, want Unauthenticated", got)
	}
	if got := handlers.Status(handlerstest.ErrNoSession); got != http.StatusUnauthorized {
		t.Errorf("ErrNoSession status = %d, want 401", got)
	}

	if got := errors.Public(handlerstest.ErrForbidden).K(); got != "errors.handlerstest.forbidden" {
		t.Errorf("ErrForbidden key = %q", got)
	}
	if got := errors.KindOf(handlerstest.ErrForbidden); got != errors.Forbidden {
		t.Errorf("ErrForbidden kind = %v, want Forbidden", got)
	}
	if got := handlers.Status(handlerstest.ErrForbidden); got != http.StatusForbidden {
		t.Errorf("ErrForbidden status = %d, want 403", got)
	}
}

func TestGuardPrepareCheckAdmit(t *testing.T) {
	pub := handlers.Route{Key: "home", Auth: handlers.Public}
	sess := handlers.Route{Key: "orders.list", Auth: handlers.Session}
	perm := handlers.Route{Key: "orders.detail", Auth: handlers.Permission, Permission: "orders.manage"}
	otherPerm := handlers.Route{Key: "users.edit", Auth: handlers.Permission, Permission: "users.manage"}
	bad := handlers.Route{Key: "bad", Auth: handlers.Auth(99)}

	req := httptest.NewRequest("GET", "/", nil)

	t.Run("no session", func(t *testing.T) {
		g := handlerstest.Guard{Granted: map[string]bool{"orders.manage": true}}
		for _, rt := range []handlers.Route{pub, sess, perm} {
			if err := g.Prepare(rt); err != nil {
				t.Errorf("Prepare(%s) = %v", rt.Key, err)
			}
		}
		if err := g.Check(req, pub); err != nil {
			t.Errorf("Check(pub) = %v", err)
		}
		if err := g.Check(req, sess); !errors.Is(err, handlerstest.ErrNoSession) {
			t.Errorf("Check(sess) = %v, want ErrNoSession", err)
		}
		if err := g.Check(req, perm); !errors.Is(err, handlerstest.ErrNoSession) {
			t.Errorf("Check(perm) = %v, want ErrNoSession", err)
		}

		admit, err := g.Admit(req)
		if err != nil {
			t.Fatalf("Admit err = %v", err)
		}
		if !admit(pub) || admit(sess) || admit(perm) {
			t.Errorf("Admit results = (%v, %v, %v), want (true, false, false)", admit(pub), admit(sess), admit(perm))
		}
	})

	t.Run("session with partial permissions", func(t *testing.T) {
		g := handlerstest.Guard{
			Session: true,
			Granted: map[string]bool{"orders.manage": true},
		}
		if err := g.Check(req, sess); err != nil {
			t.Errorf("Check(sess) = %v", err)
		}
		if err := g.Check(req, perm); err != nil {
			t.Errorf("Check(perm) = %v", err)
		}
		if err := g.Check(req, otherPerm); !errors.Is(err, handlerstest.ErrForbidden) {
			t.Errorf("Check(otherPerm) = %v, want ErrForbidden", err)
		}

		admit, err := g.Admit(req)
		if err != nil {
			t.Fatalf("Admit err = %v", err)
		}
		if !admit(pub) || !admit(sess) || !admit(perm) || admit(otherPerm) {
			t.Errorf("Admit results = (%v, %v, %v, %v)", admit(pub), admit(sess), admit(perm), admit(otherPerm))
		}
	})

	t.Run("unknown auth panics per §12.2", func(t *testing.T) {
		g := handlerstest.Guard{Session: true}
		assertPanics(t, "Prepare unknown auth", func() { _ = g.Prepare(bad) })
		assertPanics(t, "Check unknown auth", func() { _ = g.Check(req, bad) })
		admit, err := g.Admit(req)
		if err != nil {
			t.Fatalf("Admit err = %v", err)
		}
		assertPanics(t, "Admit unknown auth", func() { _ = admit(bad) })
	})
}

func TestAssertPublic(t *testing.T) {
	routes := []handlers.Route{
		{Key: "home", Auth: handlers.Public},
		{Key: "orders.list", Auth: handlers.Session},
		{Key: "health", Auth: handlers.Public},
		{Key: "orders.edit", Auth: handlers.Permission, Permission: "orders.manage"},
	}

	t.Run("exact match in any order", func(t *testing.T) {
		rec := &recordingTB{TB: t}
		handlerstest.AssertPublic(rec, routes, "health", "home")
		if len(rec.errs) != 0 {
			t.Errorf("unexpected errors: %v", rec.errs)
		}
	})

	t.Run("empty match", func(t *testing.T) {
		rec := &recordingTB{TB: t}
		handlerstest.AssertPublic(rec, []handlers.Route{{Key: "orders.list", Auth: handlers.Session}})
		if len(rec.errs) != 0 {
			t.Errorf("unexpected errors: %v", rec.errs)
		}
	})

	t.Run("unexpected and missing public routes", func(t *testing.T) {
		rec := &recordingTB{TB: t}
		handlerstest.AssertPublic(rec, routes, "home", "admin.secret")
		if len(rec.errs) != 1 {
			t.Fatalf("expected 1 error, got %v", rec.errs)
		}
		msg := rec.errs[0]
		if !strings.Contains(msg, "health") || !strings.Contains(msg, "admin.secret") {
			t.Errorf("error message %q did not list both unexpected (health) and missing (admin.secret)", msg)
		}
	})

	t.Run("unknown auth panics per §12.2", func(t *testing.T) {
		assertPanics(t, "AssertPublic unknown auth", func() {
			handlerstest.AssertPublic(&recordingTB{TB: t}, []handlers.Route{{Key: "x", Auth: handlers.Auth(99)}})
		})
	})
}

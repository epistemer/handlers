// Package authguard adapts github.com/epistemer/auth and
// github.com/epistemer/auth/access to handlers.Guard.
package authguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/epistemer/auth"
	"github.com/epistemer/auth/access"
	"github.com/epistemer/handlers"
)

type guard struct {
	a  *auth.Auth
	ac *access.Access

	permsOnce sync.Once
	perms     map[string]*access.Perm
}

// New returns a Guard over auth: session routes need a verified session
// (a.Check), permission routes a.Check with ac.Can of the declared
// permission. It panics if a or ac is nil.
func New(a *auth.Auth, ac *access.Access) handlers.Guard {
	if a == nil {
		panic("authguard: nil auth")
	}
	if ac == nil {
		panic("authguard: nil access")
	}
	return &guard{a: a, ac: ac}
}

func (g *guard) perm(key string) (*access.Perm, bool) {
	g.permsOnce.Do(func() {
		all := access.Permissions()
		m := make(map[string]*access.Perm, len(all))
		for _, p := range all {
			m[p.Key()] = p
		}
		g.perms = m
	})
	p, ok := g.perms[key]
	return p, ok
}

// Prepare verifies that rt's permission, if any, is declared in access.
// Unknown Auth values panic per interface §12.2.
func (g *guard) Prepare(rt handlers.Route) error {
	switch rt.Auth {
	case handlers.Public, handlers.Session:
		return nil
	case handlers.Permission:
		if _, ok := g.perm(rt.Permission); !ok {
			return fmt.Errorf("authguard: route %q: permission %q isn't declared", rt.Key, rt.Permission)
		}
		return nil
	default:
		panic(fmt.Sprintf("authguard: unknown auth %d", rt.Auth))
	}
}

// Check verifies the request's session and permission for rt, returning auth's
// own refusal error. Public routes fail closed with an error; unknown Auth
// values panic per interface §12.2.
func (g *guard) Check(r *http.Request, rt handlers.Route) error {
	switch rt.Auth {
	case handlers.Public:
		return errors.New("authguard: Check called for public route")
	case handlers.Session:
		if r == nil {
			return errors.New("authguard: nil request")
		}
		return g.a.Check(r.Context())
	case handlers.Permission:
		p, ok := g.perm(rt.Permission)
		if !ok {
			return fmt.Errorf("authguard: route %q: permission %q isn't declared", rt.Key, rt.Permission)
		}
		if r == nil {
			return errors.New("authguard: nil request")
		}
		return g.a.Check(r.Context(), g.ac.Can(p))
	default:
		panic(fmt.Sprintf("authguard: unknown auth %d", rt.Auth))
	}
}

// Admit returns a thread-safe check for many routes, reading the request's
// session once via auth.From and memoizing Session and per-permission checks.
// Admit returns a nil error in this version because the session is read lazily
// from the request context and permissions are checked lazily on demand;
// unexpected errors (other than auth's four refusals) are logged once with
// slog.Default at warn level and treated as refusals.
func (g *guard) Admit(r *http.Request) (func(handlers.Route) bool, error) {
	var ctx context.Context
	var hasSession bool
	if r != nil {
		ctx = r.Context()
		_, hasSession = auth.From(ctx)
	}

	var (
		mu          sync.Mutex
		sessChecked bool
		sessOK      bool
		permOK      = make(map[string]bool)
		loggedErr   bool
	)

	checkErr := func(rt handlers.Route, err error) bool {
		if err == nil {
			return true
		}
		if !isRefusal(err) && !loggedErr {
			loggedErr = true
			slog.Default().Warn("authguard: admit check failed", "route", rt.Key, "err", err)
		}
		return false
	}

	return func(rt handlers.Route) bool {
		switch rt.Auth {
		case handlers.Public:
			return true
		case handlers.Session:
			if !hasSession {
				return false
			}
			mu.Lock()
			defer mu.Unlock()
			if !sessChecked {
				sessChecked = true
				sessOK = checkErr(rt, g.a.Check(ctx))
			}
			return sessOK
		case handlers.Permission:
			if !hasSession {
				return false
			}
			p, ok := g.perm(rt.Permission)
			if !ok {
				return false
			}
			mu.Lock()
			defer mu.Unlock()
			if allowed, cached := permOK[rt.Permission]; cached {
				return allowed
			}
			allowed := checkErr(rt, g.a.Check(ctx, g.ac.Can(p)))
			permOK[rt.Permission] = allowed
			return allowed
		default:
			panic(fmt.Sprintf("authguard: unknown auth %d", rt.Auth))
		}
	}, nil
}

func isRefusal(err error) bool {
	return errors.Is(err, auth.ErrNoSession) ||
		errors.Is(err, auth.ErrNotVerified) ||
		errors.Is(err, auth.ErrStale) ||
		errors.Is(err, auth.ErrForbidden)
}

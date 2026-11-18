// Package handlerstest provides test helpers for packages using handlers.
package handlerstest

import (
	"fmt"
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/epistemer/errors"
	"github.com/epistemer/handlers"
)

// Guard is a fake: a session when Session is true, and the permissions in
// Granted. Prepare accepts every route; Check refuses with ErrNoSession or
// ErrForbidden.
type Guard struct {
	Session bool
	Granted map[string]bool
}

var (
	// ErrNoSession is returned when a non-public route is accessed without a session.
	ErrNoSession = errors.Key("errors.handlerstest.no_session", "no session", errors.Unauthenticated)
	// ErrForbidden is returned when a permission route is accessed without its permission.
	ErrForbidden = errors.Key("errors.handlerstest.forbidden", "forbidden", errors.Forbidden)
)

// Prepare accepts every route with a known Auth value.
func (g Guard) Prepare(rt handlers.Route) error {
	switch rt.Auth {
	case handlers.Public, handlers.Session, handlers.Permission:
		return nil
	default:
		panic(fmt.Sprintf("handlerstest: unknown auth %d", rt.Auth))
	}
}

// Check admits Public routes, requires Session for Session routes, and requires
// Session and Granted[rt.Permission] for Permission routes.
func (g Guard) Check(_ *http.Request, rt handlers.Route) error {
	switch rt.Auth {
	case handlers.Public:
		return nil
	case handlers.Session:
		if !g.Session {
			return ErrNoSession
		}
		return nil
	case handlers.Permission:
		if !g.Session {
			return ErrNoSession
		}
		if !g.Granted[rt.Permission] {
			return ErrForbidden
		}
		return nil
	default:
		panic(fmt.Sprintf("handlerstest: unknown auth %d", rt.Auth))
	}
}

// Admit returns a check mirroring Guard.Check with a nil error.
func (g Guard) Admit(*http.Request) (func(handlers.Route) bool, error) {
	snap := Guard{
		Session: g.Session,
		Granted: maps.Clone(g.Granted),
	}
	return func(rt handlers.Route) bool {
		return snap.Check(nil, rt) == nil
	}, nil
}

// AssertPublic fails t unless the public routes of routes are exactly keys.
func AssertPublic(t testing.TB, routes []handlers.Route, keys ...string) {
	t.Helper()
	got := make(map[string]bool)
	for _, rt := range routes {
		switch rt.Auth {
		case handlers.Public:
			got[rt.Key] = true
		case handlers.Session, handlers.Permission:
		default:
			panic(fmt.Sprintf("handlerstest: unknown auth %d", rt.Auth))
		}
	}
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	var unexpected, missing []string
	for k := range got {
		if !want[k] {
			unexpected = append(unexpected, k)
		}
	}
	for k := range want {
		if !got[k] {
			missing = append(missing, k)
		}
	}
	if len(unexpected) == 0 && len(missing) == 0 {
		return
	}
	slices.Sort(unexpected)
	slices.Sort(missing)
	t.Errorf("handlerstest.AssertPublic: public routes mismatch: unexpected=%v missing=%v", unexpected, missing)
}

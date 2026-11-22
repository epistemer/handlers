package handlers

import (
	"context"
	"fmt"

	"github.com/epistemer/i18n"
)

// Surface identifies whether a route serves HTML pages or a JSON API.
type Surface uint8

const (
	// HTML is the server-rendered admin surface.
	HTML Surface = iota + 1
	// JSON is the OpenAPI-backed JSON surface.
	JSON
)

// String returns "html" or "json". It panics on an unknown Surface.
func (s Surface) String() string {
	switch s {
	case HTML:
		return "html"
	case JSON:
		return "json"
	default:
		panic(fmt.Sprintf("handlers: unknown surface %d", s))
	}
}

// Auth identifies the authentication and authorization level required by a route.
type Auth uint8

const (
	// Public routes require no session or permission.
	Public Auth = iota + 1
	// Session routes require a verified session.
	Session
	// Permission routes require a session that holds the route's permission.
	Permission
)

// String returns "public", "session", or "permission". It panics on an unknown Auth.
func (a Auth) String() string {
	switch a {
	case Public:
		return "public"
	case Session:
		return "session"
	case Permission:
		return "permission"
	default:
		panic(fmt.Sprintf("handlers: unknown auth %d", a))
	}
}

// Route describes one registered HTTP route in a generated route table.
type Route struct {
	Key        string
	Method     string // GET, POST, PUT, PATCH, DELETE
	Prefix     string // the package's mount prefix
	Pattern    string // the path as declared, without the prefix
	Surface    Surface
	Auth       Auth
	Permission string    // the permission's key when Auth is Permission
	Label      i18n.TKer // route.<key>
	View       bool
	Searchable bool
	Entity     string // a route key, or ""
	MaxBody    int64  // bytes; 0: the registration's limit
	Operation  string // the operationId on the JSON surface
	Params     []Param
}

// Param describes one path parameter of a Route.
type Param struct {
	Name  string    // the wildcard's name
	Rest  bool      // {name...}
	Label i18n.TKer // route_param.<key>.<name>
}

type routeKey struct{}

// From returns the route of the request ctx belongs to.
func From(ctx context.Context) (Route, bool) {
	if ctx == nil {
		return Route{}, false
	}
	rt, ok := ctx.Value(routeKey{}).(Route)
	return rt, ok
}

// WithRoute returns a copy of ctx carrying rt.
func WithRoute(ctx context.Context, rt Route) context.Context {
	return context.WithValue(ctx, routeKey{}, rt)
}

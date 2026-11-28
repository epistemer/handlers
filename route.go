package handlers

import (
	"context"
	"fmt"
	"strconv"

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
	MaxBody    int64  // bytes; 0: the registration's limit
	Operation  string // the operationId on the JSON surface
	Params     []Param
}

// Param describes one path parameter of a Route.
type Param struct {
	Name  string    // the wildcard's name
	Rest  bool      // {name...}
	Label i18n.TKer // route_param.<key>.<name>
	Kind  ParamKind // how the binder parses it (A9)
	// Bits is the width of a KindInt or KindUint: 8, 16, 32, 64, or
	// strconv.IntSize for int and uint (0 reads as strconv.IntSize, as in
	// strconv); 0 for the other kinds (A9).
	Bits int
	// From is the key of the route that suggests the parameter's values,
	// from //h:param <name> from <key>; "" when it has none (A9).
	From string
	// Canon reads a KindText value as the binder does and returns it as
	// MarshalText writes it, unescaped. Generated; nil unless Kind is
	// KindText (A9).
	Canon func(string) (string, error)
}

// ParamKind is how a path parameter is parsed: routes/v1's parse (A9).
type ParamKind uint8

const (
	// KindString is a string or a named string type. A Rest parameter
	// is always KindString.
	KindString ParamKind = iota
	// KindBool is a bool or a named bool type.
	KindBool
	// KindInt is int, int8 … int64 or a named type over one; Bits says which.
	KindInt
	// KindUint is uint, uint8 … uint64 or a named type over one; Bits says which.
	KindUint
	// KindText is a type whose pointer implements encoding.TextUnmarshaler
	// and which implements encoding.TextMarshaler.
	KindText
)

// String is the kind's name in routes/v1's terms: "string", "bool", "int",
// "uint" or "text".
func (k ParamKind) String() string {
	switch k {
	case KindString:
		return "string"
	case KindBool:
		return "bool"
	case KindInt:
		return "int"
	case KindUint:
		return "uint"
	case KindText:
		return "text"
	}
	return "ParamKind(" + strconv.Itoa(int(k)) + ")"
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

package handlers

import (
	"net/url"
	"slices"
	"strings"
)

// Link is a route's URL, as a generated builder makes it: the route it
// goes to and the address with its parameters written in (ADR 0005,
// amendment A4). It satisfies ui's Link, interface{ URL() string }, so a
// builder's result passes to ui's elements and actions as it is. Links are
// comparable.
type Link struct {
	route *Route // in the generated table; nil for the zero Link
	url   string
}

// NewLink is for generated code: the link to rt at url.
func NewLink(rt *Route, url string) Link { return Link{route: rt, url: url} }

// URL is the address: the prefix and the path, and a query when With
// added one.
func (l Link) URL() string { return l.url }

// String is URL, for fmt and logs.
func (l Link) String() string { return l.url }

// Method is the route's method: GET, POST, …; "" for the zero Link.
func (l Link) Method() string {
	if l.route == nil {
		return ""
	}
	return l.route.Method
}

// Route is the route the link goes to; the zero Route for the zero Link.
// It is a copy: changing it, its Params included, leaves the table alone.
func (l Link) Route() Route {
	if l.route == nil {
		return Route{}
	}
	rt := *l.route
	rt.Params = slices.Clone(rt.Params)
	return rt
}

// linkRoute is the route behind a link, for Gate. Unexported, so only a
// Link has it, and a type that embeds one gets it by promotion.
func (l Link) linkRoute() *Route { return l.route }

// With returns the link with q as its query, replacing any, before a
// fragment if there is one; a q that encodes to nothing removes it.
func (l Link) With(q url.Values) Link {
	rest, frag, hasFrag := strings.Cut(l.url, "#")
	path, _, _ := strings.Cut(rest, "?")
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	if hasFrag {
		path += "#" + frag
	}
	l.url = path
	return l
}

// Gate adapts a guard's check, from Guard.Admit, to ui's actions gate: a
// Link, a *Link or a type embedding one asks admit about its route; any
// other link (ui.URL, an external address, the zero Link) has no route to
// ask about, and is admitted. The gate decides what a page shows, never
// what is allowed: the route's guard still runs on every request. A nil
// admit panics here, not at the first link.
func Gate(admit func(Route) bool) func(interface{ URL() string }) bool {
	if admit == nil {
		panic("handlers: Gate(nil): pass the check Guard.Admit returned")
	}
	return func(l interface{ URL() string }) bool {
		lr, ok := l.(interface{ linkRoute() *Route })
		if !ok {
			return true
		}
		if p, isPtr := l.(*Link); isPtr && p == nil {
			return true
		}
		rt := lr.linkRoute()
		if rt == nil {
			return true
		}
		return admit(*rt)
	}
}

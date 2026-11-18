package handlers

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/epistemer/i18n"
)

// Table holds registered routes for lookup and search.
type Table struct {
	routes []Route
	byKey  map[string]Route
}

// NewTable combines route slices into a single Table, preserving their order.
// It panics if a key appears in two tables.
func NewTable(routes ...[]Route) *Table {
	var total int
	for _, slice := range routes {
		total += len(slice)
	}
	all := make([]Route, 0, total)
	byKey := make(map[string]Route, total)
	for _, slice := range routes {
		for _, rt := range slice {
			if _, ok := byKey[rt.Key]; ok {
				panic(fmt.Sprintf("handlers: NewTable: key %q in two tables", rt.Key))
			}
			clone := rt
			if rt.Params != nil {
				clone.Params = slices.Clone(rt.Params)
			}
			byKey[rt.Key] = clone
			all = append(all, clone)
		}
	}
	return &Table{
		routes: all,
		byKey:  byKey,
	}
}

// ByKey returns the route with the given key and whether it was found.
func (t *Table) ByKey(key string) (Route, bool) {
	if t == nil {
		return Route{}, false
	}
	rt, ok := t.byKey[key]
	if ok && rt.Params != nil {
		rt.Params = slices.Clone(rt.Params)
	}
	return rt, ok
}

// Routes returns a copy of the routes in the table, in the order given.
func (t *Table) Routes() []Route {
	if t == nil || len(t.routes) == 0 {
		return []Route{}
	}
	out := make([]Route, len(t.routes))
	for i, rt := range t.routes {
		out[i] = rt
		if rt.Params != nil {
			out[i].Params = slices.Clone(rt.Params)
		}
	}
	return out
}

// Query specifies filters for searching routes in a Table.
type Query struct {
	Text  string           // matched, ignoring case, against the text in the request's locale and the source text
	Only  func(Route) bool // optional, e.g. searchable pages
	Limit int              // 0 means no limit
}

// Hit represents a route matched during search, with its label in the request's locale.
type Hit struct {
	Route Route
	Text  string // the label in the request's locale
}

// Search searches for routes in t matching q in the request's locale.
// It calls g.Admit(r) once (or admits only public routes if g is nil).
// Hits are returned in key order up to q.Limit. Search returns Admit's error.
func (t *Table) Search(r *http.Request, g Guard, tr *i18n.Translator, q Query) ([]Hit, error) {
	var admit func(Route) bool
	if g != nil {
		var err error
		admit, err = g.Admit(r)
		if err != nil {
			return nil, err
		}
	} else {
		admit = func(rt Route) bool {
			return rt.Auth == Public
		}
	}
	if admit == nil {
		admit = func(rt Route) bool {
			return rt.Auth == Public
		}
	}

	if t == nil {
		return nil, nil
	}
	if tr == nil {
		panic("handlers: nil translator")
	}

	var code string
	if r != nil {
		code = i18n.Code(r.Context()).Code
	}
	if code == "" {
		code = tr.Default().Code
	}

	target := strings.ToLower(q.Text)
	match := func(en i18n.Entry) bool {
		routeKey, ok := strings.CutPrefix(en.Key, "route.")
		if !ok {
			return false
		}
		rt, ok := t.byKey[routeKey]
		if !ok {
			return false
		}
		if target != "" {
			if !strings.Contains(strings.ToLower(en.Text), target) &&
				!strings.Contains(strings.ToLower(en.Source), target) {
				return false
			}
		}
		if q.Only != nil && !q.Only(rt) {
			return false
		}
		if !admit(rt) {
			return false
		}
		return true
	}

	iq := i18n.Query{
		Prefix:   "route.",
		Contains: "",
		Match:    match,
	}
	if q.Limit > 0 {
		iq.Limit = q.Limit
	}

	entries, err := tr.Search(code, iq)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	hits := make([]Hit, len(entries))
	for i, en := range entries {
		routeKey, _ := strings.CutPrefix(en.Key, "route.")
		rt := t.byKey[routeKey]
		if rt.Params != nil {
			rt.Params = slices.Clone(rt.Params)
		}
		hits[i] = Hit{
			Route: rt,
			Text:  en.Text,
		}
	}
	return hits, nil
}

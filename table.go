package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/epistemer/i18n"
)

// Table holds registered routes for lookup and search.
type Table struct {
	routes []Route
	byKey  map[string]Route
	index  map[string]int // a key's position in routes, for links into the table
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
	index := make(map[string]int, total)
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
			index[rt.Key] = len(all)
			all = append(all, clone)
		}
	}
	return &Table{
		routes: all,
		byKey:  byKey,
		index:  index,
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
	// Link is the route's link when it has no parameters, else the zero
	// Link (A9).
	Link Link
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
		if len(rt.Params) == 0 {
			if path, ok := fillPattern(rt.Pattern, nil); ok {
				hits[i].Link = NewLink(&t.routes[t.index[routeKey]], rt.Prefix+path)
			}
		}
	}
	return hits, nil
}

// Link returns the link to the route with this key, its parameters taken
// from v by name, each parsed as the binder parses it and written as the
// route's builder writes it (A9). Values for names that aren't parameters
// are ignored, and the link has no query. An unknown key is ErrNotFound. A
// parameter whose value is missing, given more than once, empty, or that
// doesn't parse is ErrBadParam with the parameter's label; so is a Rest
// value that starts or ends with "/" or has an empty segment. Link consults
// no guard: a caller that shows the link to someone checks it first.
func (t *Table) Link(key string, v url.Values) (Link, error) {
	if t == nil {
		return Link{}, ErrNotFound
	}
	i, ok := t.index[key]
	if !ok {
		return Link{}, ErrNotFound
	}
	rt := &t.routes[i]
	vals := make(map[string]string, len(rt.Params))
	for _, p := range rt.Params {
		s, err := linkValue(p, v[p.Name])
		if err != nil {
			return Link{}, err
		}
		vals[p.Name] = s
	}
	path, ok := fillPattern(rt.Pattern, vals)
	if !ok {
		return Link{}, ErrBadParam.With(rt.Label)
	}
	return NewLink(rt, rt.Prefix+path), nil
}

// linkValue is one parameter's value as its builder writes it, escaped.
func linkValue(p Param, given []string) (string, error) {
	bad := ErrBadParam.With(p.Label)
	if len(given) != 1 || given[0] == "" {
		return "", bad
	}
	s := given[0]
	if p.Rest {
		if p.Kind != KindString || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") || strings.Contains(s, "//") {
			return "", bad
		}
		return Rest(s), nil
	}
	switch p.Kind {
	case KindBool:
		b, err := strconv.ParseBool(s)
		if err != nil {
			return "", bad
		}
		return strconv.FormatBool(b), nil
	case KindInt:
		n, err := strconv.ParseInt(s, 10, p.Bits)
		if err != nil {
			return "", bad
		}
		return strconv.FormatInt(n, 10), nil
	case KindUint:
		n, err := strconv.ParseUint(s, 10, p.Bits)
		if err != nil {
			return "", bad
		}
		return strconv.FormatUint(n, 10), nil
	case KindText:
		if p.Canon == nil {
			return "", bad
		}
		c, err := p.Canon(s)
		if err != nil || c == "" {
			return "", bad
		}
		return Segment(c), nil
	case KindString:
		return Segment(s), nil
	}
	return "", bad
}

// fillPattern writes vals, already escaped, into pattern's wildcards; {$}
// writes nothing. It reports false for a wildcard vals has no value for, or
// a brace that isn't closed: a hand-built route whose pattern and
// parameters disagree.
func fillPattern(pattern string, vals map[string]string) (string, bool) {
	var b strings.Builder
	for {
		open := strings.IndexByte(pattern, '{')
		if open < 0 {
			b.WriteString(pattern)
			return b.String(), true
		}
		end := strings.IndexByte(pattern[open:], '}')
		if end < 0 {
			return "", false
		}
		b.WriteString(pattern[:open])
		if raw := pattern[open+1 : open+end]; raw != "$" {
			v, ok := vals[strings.TrimSuffix(raw, "...")]
			if !ok {
				return "", false
			}
			b.WriteString(v)
		}
		pattern = pattern[open+end+1:]
	}
}

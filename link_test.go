package handlers_test

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/epistemer/handlers"
)

var table = []handlers.Route{
	{Key: "orders.list", Method: "GET", Prefix: "/admin", Pattern: "/orders", Params: []handlers.Param{}},
	{Key: "orders.refund", Method: "POST", Prefix: "/admin", Pattern: "/orders/{id}/refund"},
}

type plain string

// action embeds a link, as a UI type might: the gate still asks.
type action struct {
	handlers.Link
}

func (p plain) URL() string { return string(p) }

func TestLink(t *testing.T) {
	l := handlers.NewLink(&table[1], "/admin/orders/7/refund")
	if l.URL() != "/admin/orders/7/refund" || l.String() != l.URL() || fmt.Sprint(l) != l.URL() {
		t.Errorf("URL %q String %q", l.URL(), l.String())
	}
	if l.Method() != "POST" || l.Route().Key != "orders.refund" {
		t.Errorf("Method %q Route %q", l.Method(), l.Route().Key)
	}
	list := handlers.NewLink(&table[0], "/admin/orders?old=1")
	q := list.With(url.Values{"status": {"paid"}, "page": {"2"}})
	if q.URL() != "/admin/orders?page=2&status=paid" || q.Route().Key != "orders.list" {
		t.Errorf("With: %q", q.URL())
	}
	if list.With(nil).URL() != "/admin/orders" {
		t.Errorf("With(nil) keeps the query: %q", list.With(nil).URL())
	}
	if list.URL() != "/admin/orders?old=1" {
		t.Error("With changed the original")
	}
	frag := handlers.NewLink(&table[0], "/admin/orders?old=1#top")
	if got := frag.With(url.Values{"a": {"1"}}).URL(); got != "/admin/orders?a=1#top" {
		t.Errorf("With and a fragment: %q", got)
	}
	if got := list.With(url.Values{"empty": {}}).URL(); got != "/admin/orders" {
		t.Errorf("With of values that encode to nothing: %q", got)
	}
	rt := handlers.NewLink(&table[0], "/admin/orders").Route()
	rt.Params = append(rt.Params, handlers.Param{Name: "x"})
	if len(table[0].Params) != 0 {
		t.Error("Route() let a caller change the table")
	}
	// Comparable: a link is a map key and ==.
	a, b := handlers.NewLink(&table[0], "/admin/orders"), handlers.NewLink(&table[0], "/admin/orders")
	if a != b {
		t.Error("equal links differ")
	}
	_ = map[handlers.Link]bool{l: true}
	var zero handlers.Link
	if zero.Method() != "" || zero.Route().Key != "" || zero.URL() != "" {
		t.Error("the zero Link")
	}
}

func TestGate(t *testing.T) {
	asked := []string{}
	gate := handlers.Gate(func(rt handlers.Route) bool {
		asked = append(asked, rt.Key)
		return rt.Key != "orders.refund"
	})
	refund := handlers.NewLink(&table[1], "/admin/orders/7/refund")
	list := handlers.NewLink(&table[0], "/admin/orders")
	var nilLink *handlers.Link
	for name, c := range map[string]struct {
		l    interface{ URL() string }
		want bool
	}{
		"a refused route":        {refund, false},
		"an admitted route":      {list, true},
		"a pointer to a link":    {&refund, false},
		"a nil pointer":          {nilLink, true},
		"a link that's no route": {plain("/docs"), true},
		"the zero Link":          {handlers.Link{}, true},
		"a type embedding one":   {action{Link: refund}, false},
	} {
		if got := gate(c.l); got != c.want {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
	if len(asked) != 4 {
		t.Errorf("admit asked %d times, want 4 (route links only): %v", len(asked), asked)
	}
	defer func() {
		if recover() == nil {
			t.Error("Gate(nil) didn't panic")
		}
	}()
	handlers.Gate(nil)
}

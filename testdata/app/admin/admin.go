package admin

import (
	"fmt"
	"net/http"

	"app/ids"
	"github.com/epistemer/handlers"
)

// H implements the adminroutes.Handler interface.
type H struct {
	LastRoute handlers.Route
}

//h:key dashboard
//h:method GET
//h:path /
//h:desc Dashboard
//h:auth public
//h:view
func (h *H) Dashboard(w http.ResponseWriter, r *http.Request) error {
	if rt, ok := handlers.From(r.Context()); ok {
		h.LastRoute = rt
	}
	_, err := fmt.Fprint(w, "dashboard")
	return err
}

//h:key orders.list
//h:method GET
//h:path /orders
//h:desc List orders
//h:auth session
//h:view
//h:search
func (h *H) Orders(w http.ResponseWriter, r *http.Request) error {
	if rt, ok := handlers.From(r.Context()); ok {
		h.LastRoute = rt
	}
	_, err := fmt.Fprint(w, "orders")
	return err
}

//h:key orders.detail
//h:method GET
//h:path /orders/{id}/{tok}
//h:desc Order detail
//h:perm orders.manage
//h:entity orders.list
//h:param id label Order ID
//h:param tok label Access token
func (h *H) Order(w http.ResponseWriter, r *http.Request, id ids.OrderID, tok ids.Token) error {
	if rt, ok := handlers.From(r.Context()); ok {
		h.LastRoute = rt
	}
	_, err := fmt.Fprintf(w, "order:%d:%s", id, tok.Raw)
	return err
}

//h:key orders.cancel
//h:method POST
//h:path /orders/{id}/cancel
//h:desc Cancel order
//h:perm orders.manage
//h:body 20MiB
//h:param id label Order ID
func (h *H) OrderCancel(w http.ResponseWriter, r *http.Request, id ids.OrderID) error {
	if rt, ok := handlers.From(r.Context()); ok {
		h.LastRoute = rt
	}
	_, err := fmt.Fprintf(w, "cancel:%d", id)
	return err
}

//h:key files.serve
//h:method GET
//h:path /files/{path...}
//h:desc Serve files
//h:auth public
//h:param path label File path
func (h *H) Files(w http.ResponseWriter, r *http.Request, path string) error {
	if rt, ok := handlers.From(r.Context()); ok {
		h.LastRoute = rt
	}
	_, err := fmt.Fprintf(w, "file:%s", path)
	return err
}

//h:key flags.set
//h:method POST
//h:path /flags/{ok}/{n}
//h:desc Set flags
//h:auth session
//h:param ok label Enabled
//h:param n label Count
func (h *H) Flags(w http.ResponseWriter, r *http.Request, ok bool, n uint16) error {
	if rt, ok := handlers.From(r.Context()); ok {
		h.LastRoute = rt
	}
	_, err := fmt.Fprintf(w, "flags:%t:%d", ok, n)
	return err
}

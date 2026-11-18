package shop

import "net/http"

type H struct{}

// p[orderID]:label Order ID
//
//h:key orders.show
//h:method GET
//h:path /orders/{orderID}
//h:desc Show order
func (h *H) Show(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

//h:key orders.show
//h:method GET
//h:path /orders/{orderID}
//h:desc Duplicate order
func (h *H) ShowDup(w http.ResponseWriter, r *http.Request, orderID int64) error { return nil }

//h:key orders.badpath
//h:method GET
//h:path orders/list
//h:desc Bad path
func (h *H) BadPath(w http.ResponseWriter, r *http.Request) error { return nil }

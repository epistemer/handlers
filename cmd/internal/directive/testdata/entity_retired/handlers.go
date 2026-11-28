package shop

import "net/http"

type H struct{}

//h:key items.list
//h:method GET
//h:path /items
//h:desc Items
//h:entity items.list
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }

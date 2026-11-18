package shop

import "net/http"

type H struct{}

//h:key items.list
//h:method GET
//h:path /items
//h:desc Items
//h:cache 60s
func (h *H) Items(w http.ResponseWriter, r *http.Request) error { return nil }

package shop

import "net/http"

type H struct{}

//h:key Items.List
//h:method GET
//h:path /items
//h:desc Items
//h:perm bad-perm
func (h *H) List(w http.ResponseWriter, r *http.Request) error { return nil }

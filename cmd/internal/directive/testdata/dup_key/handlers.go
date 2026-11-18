package shop

import "net/http"

type H struct{}

//h:key items.list
//h:method GET
//h:path /items
//h:desc Items
func (h *H) One(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key items.list
//h:method GET
//h:path /items2
//h:desc Items 2
func (h *H) Two(w http.ResponseWriter, r *http.Request) error { return nil }

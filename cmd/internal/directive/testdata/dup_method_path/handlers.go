package shop

import "net/http"

type H struct{}

//h:key items.one
//h:method GET
//h:path /items
//h:desc One
func (h *H) One(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key items.two
//h:method GET
//h:path /items
//h:desc Two
func (h *H) Two(w http.ResponseWriter, r *http.Request) error { return nil }

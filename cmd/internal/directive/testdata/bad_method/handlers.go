package shop

import "net/http"

type H struct{}

//h:key items.head
//h:method HEAD
//h:path /items
//h:desc Head items
func (h *H) Head(w http.ResponseWriter, r *http.Request) error { return nil }

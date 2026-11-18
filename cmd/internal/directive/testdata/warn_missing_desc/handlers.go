package shop

import "net/http"

type H struct{}

//h:key categories.profile
//h:method GET
//h:path /categories
func (h *H) Profile(w http.ResponseWriter, r *http.Request) error { return nil }

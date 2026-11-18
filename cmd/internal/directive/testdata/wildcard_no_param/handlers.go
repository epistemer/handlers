package shop

import "net/http"

type H struct{}

//h:key items.show
//h:method GET
//h:path /items/{id}
//h:desc Show
func (h *H) Show(w http.ResponseWriter, r *http.Request) error { return nil }

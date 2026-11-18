package shop

import "net/http"

type H struct{}

//h:key items.show
//h:method GET
//h:path /items/{id}
//h:desc Show item
//h:param id title Item ID
//h:param other label Other
func (h *H) Show(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

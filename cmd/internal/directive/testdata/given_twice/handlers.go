package shop

import "net/http"

type H struct{}

//h:key items.show
//h:key items.other
//h:method GET
//h:path /items/{id}
//h:desc Show item
//h:param id label Item ID
//h:param id label Duplicate ID
func (h *H) Show(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

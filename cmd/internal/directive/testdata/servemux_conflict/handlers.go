package shop

import "net/http"

type H struct{}

//h:key items.byid
//h:method GET
//h:path /items/{id}
//h:desc By ID
func (h *H) ByID(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

//h:key items.byname
//h:method GET
//h:path /items/{name}
//h:desc By Name
func (h *H) ByName(w http.ResponseWriter, r *http.Request, name string) error { return nil }

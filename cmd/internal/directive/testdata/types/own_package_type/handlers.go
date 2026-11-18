package ownpkg

import "net/http"

type LocalID int64

type H struct{}

//h:key items.show
//h:method GET
//h:path /items/{id}
//h:desc Show
func (h *H) Show(w http.ResponseWriter, r *http.Request, id LocalID) error {
	return nil
}

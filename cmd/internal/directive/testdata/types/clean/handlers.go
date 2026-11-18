package clean

import (
	"net/http"
	"toy/ext"
)

type H struct{}

//h:key items.show
//h:method GET
//h:path /items/{id}/{flag}/{u}/{cid}/{rest...}
//h:desc Show
func (h *H) Show(w http.ResponseWriter, r *http.Request, id int64, flag bool, u uint32, cid ext.CustomID, rest ext.Slug) error {
	return nil
}

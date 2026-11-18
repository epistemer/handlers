package typecheckerr

import "net/http"

type H struct{}

//h:key items.show
//h:method GET
//h:path /items/{id}
//h:desc Show
func (h *H) Show(w http.ResponseWriter, r *http.Request, id UndefinedType) error {
	return nil
}

package shop

import "net/http"

type H struct{}

//h:key categories.delete
//h:method DELETE
//h:path /categories/{id}
//h:desc Delete category
//h:perm categories.admin
func (h *H) Delete(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

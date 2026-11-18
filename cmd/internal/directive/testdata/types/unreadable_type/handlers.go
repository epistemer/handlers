package unreadable

import "net/http"

type H struct{}

//h:key items.rate
//h:method GET
//h:path /items/{score}/{ptr}
//h:desc Rate
func (h *H) Rate(w http.ResponseWriter, r *http.Request, score float64, ptr uintptr) error {
	return nil
}

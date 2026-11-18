package shop

import "net/http"

type H struct{}

//h:desc Incomplete
func (h *H) Incomplete(w http.ResponseWriter, r *http.Request) error { return nil }

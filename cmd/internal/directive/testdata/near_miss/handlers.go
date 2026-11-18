package shop

import "net/http"

type H struct{}

// h:key items.list
// H:method GET
// h :path /items
// p[id]:label Item ID
// p[slug]:label Slug
// h key prose should not match
// Handler serves items
func (h *H) Items(w http.ResponseWriter, r *http.Request) error { return nil }

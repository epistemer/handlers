package shop

import "net/http"

type H struct{}

//h:key a.one
//h:method GET
//h:path items
//h:desc One
func (h *H) One(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.two
//h:method GET
//h:path /items/
//h:desc Two
func (h *H) Two(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.three
//h:method GET
//h:path /items/{$}
//h:desc Three
func (h *H) Three(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.four
//h:method GET
//h:path /items/{id}/{id}
//h:desc Four
func (h *H) Four(w http.ResponseWriter, r *http.Request, id int64) error { return nil }

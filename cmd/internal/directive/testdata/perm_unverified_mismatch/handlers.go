package shop

import "net/http"

type H struct{}

//h:key a.one
//h:method GET
//h:path /one
//h:desc One
//h:auth unverified
//h:perm shared.perm
func (h *H) One(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.two
//h:method GET
//h:path /two
//h:desc Two
//h:perm shared.perm
func (h *H) Two(w http.ResponseWriter, r *http.Request) error { return nil }

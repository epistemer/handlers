package shop

import "net/http"

type A struct{}
type B struct{}

//h:key a.one
//h:method GET
//h:path /one
//h:desc One
func (a *A) One(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key b.two
//h:method GET
//h:path /two
//h:desc Two
func (b *B) Two(w http.ResponseWriter, r *http.Request) error { return nil }

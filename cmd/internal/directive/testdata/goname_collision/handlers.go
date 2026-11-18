package shop

import "net/http"

type H struct{}

//h:key a.b
//h:method GET
//h:path /ab
//h:desc AB
func (h *H) AB(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a_b
//h:method GET
//h:path /ab2
//h:desc AB 2
func (h *H) AB2(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key c.one
//h:method GET
//h:path /c/{bar}
//h:desc C One
func (h *H) Foo(w http.ResponseWriter, r *http.Request, bar string) error { return nil }

//h:key c.two
//h:method GET
//h:path /c2
//h:desc C Two
func (h *H) FooBar(w http.ResponseWriter, r *http.Request) error { return nil }

package shop

import "net/http"

type H struct{}

//h:key a.func
//h:method GET
//h:path /func
//h:desc Func
func TopLevel(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.unexp
//h:method GET
//h:path /unexp
//h:desc Unexported
func (h *H) unexported(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.args
//h:method GET
//h:path /args
//h:desc Bad args
func (h *H) BadArgs(w http.ResponseWriter) error { return nil }

//h:key a.ret
//h:method GET
//h:path /ret
//h:desc Bad ret
func (h *H) BadRet(w http.ResponseWriter, r *http.Request) {}

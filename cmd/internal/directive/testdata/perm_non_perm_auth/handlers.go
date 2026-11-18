package shop

import "net/http"

type H struct{}

//h:key a.pub
//h:method GET
//h:path /pub
//h:desc Public
//h:auth public
//h:perm shared.perm
func (h *H) Pub(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.sess
//h:method GET
//h:path /sess
//h:desc Session
//h:auth session
//h:perm shared.perm
func (h *H) Sess(w http.ResponseWriter, r *http.Request) error { return nil }

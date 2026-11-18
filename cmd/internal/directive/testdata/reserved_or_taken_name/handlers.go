package shop

import "net/http"

type H struct{}

//h:key a.one
//h:method GET
//h:path /one
//h:desc One
//h:perm admin.users
func (h *H) Routes(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.two
//h:method GET
//h:path /two
//h:desc Two
//h:perm admin.users
func (h *H) PermAdminUsers(w http.ResponseWriter, r *http.Request) error { return nil }

//h:key a.three
//h:method GET
//h:path /three
//h:desc Three
func (h *H) L10nRoutes(w http.ResponseWriter, r *http.Request) error { return nil }

package shop

import "net/http"

type H struct{}

//h:key categories.list
//h:method get
//h:path /categories
//h:desc List categories
//h:auth public
//h:view
//h:search true
func (h *H) ListCategories(w http.ResponseWriter, r *http.Request) error {
	return nil
}

//h:key categories.profile
//h:method GET
//h:path /categories/{id}
//h:desc Category detail
//h:perm categories.manage
//h:param id from categories.list label Category ID
func (h *H) Category(w http.ResponseWriter, r *http.Request, id int64) error {
	return nil
}

//h:key categories.update
//h:method POST
//h:path /categories/{id}/files/{rest...}
//h:desc Update category files
//h:perm categories.manage
//h:param id label Category ID
//h:param rest label File Path
func (h *H) UpdateCategoryFiles(w http.ResponseWriter, r *http.Request, id int64, rest string) error {
	return nil
}

//h:key session.me
//h:method GET
//h:path /me
//h:desc Current session
//h:auth session
func (h *H) Me(w http.ResponseWriter, r *http.Request) error {
	return nil
}

//h:key auth.verify
//h:method POST
//h:path /verify
//h:desc Verify account
//h:auth unverified
func (h *H) Verify(w http.ResponseWriter, r *http.Request) error {
	return nil
}

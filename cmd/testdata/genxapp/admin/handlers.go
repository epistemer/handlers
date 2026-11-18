package admin

import "net/http"

type H struct{}

//h:key dashboard
//h:method GET
//h:path /
//h:desc Dashboard
//h:auth public
//h:view
func (h *H) Dashboard(w http.ResponseWriter, r *http.Request) error {
	return nil
}

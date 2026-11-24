package unusedroutesimport

import (
	"net/http"

	"toy/unused_routes_import/adminroutes"
)

type H struct{}

//h:key events.list
//h:method GET
//h:path /events
//h:desc List events
func (h *H) Events(w http.ResponseWriter, r *http.Request) error {
	return nil
}

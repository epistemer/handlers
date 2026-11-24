package missingout

import (
	"net/http"

	"toy/missing_out/adminroutes"
)

type H struct{}

//h:key events.list
//h:method GET
//h:path /events
//h:desc List events
func (h *H) Events(w http.ResponseWriter, r *http.Request) error {
	_ = adminroutes.EventsList()
	return nil
}

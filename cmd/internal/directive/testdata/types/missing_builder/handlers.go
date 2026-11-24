package missingbuilder

import (
	"net/http"

	"toy/missing_builder/adminroutes"
)

type H struct{}

//h:key events.delete
//h:method POST
//h:path /events/{id}/delete
//h:desc Delete event
func (h *H) EventsDelete(w http.ResponseWriter, r *http.Request, id int64) error {
	_ = adminroutes.EventsDelete(id)
	return nil
}

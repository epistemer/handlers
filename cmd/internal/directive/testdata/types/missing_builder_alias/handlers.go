package missingbuilderalias

import (
	"net/http"

	ar "toy/missing_builder_alias/adminroutes"
)

type H struct{}

//h:key events.delete
//h:method POST
//h:path /events/{id}/delete
//h:desc Delete event
func (h *H) EventsDelete(w http.ResponseWriter, r *http.Request, id int64) error {
	_ = ar.EventsDelete(id)
	return nil
}

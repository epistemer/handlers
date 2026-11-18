package restnonstring

import "net/http"

type H struct{}

//h:key files.get
//h:method GET
//h:path /files/{rest...}
//h:desc Get file
func (h *H) Get(w http.ResponseWriter, r *http.Request, rest int64) error {
	return nil
}

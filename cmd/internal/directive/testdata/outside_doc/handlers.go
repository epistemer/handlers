package shop

import "net/http"

type H struct{}

//h:key stray.dir

func helper() {}

func (h *H) Ok(w http.ResponseWriter, r *http.Request) error {
	//h:method GET
	return nil
}

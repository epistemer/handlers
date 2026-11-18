package aliases

import (
	"net/http"

	aext "golden/a/ext"
	bext "golden/b/ext"
)

type H struct{}

//h:key orders.show
//h:method GET
//h:path /orders/{id}/{tok}
//h:desc Show order
//h:param id label Order ID
func (h *H) Show(w http.ResponseWriter, r *http.Request, id aext.OrderID, tok bext.Token) error {
	return nil
}

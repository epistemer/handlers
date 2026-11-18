// Code generated from labels by github.com/epistemer/i18n. DO NOT EDIT.

package apiroutes

import "github.com/epistemer/i18n"

var (
	L10nSignIn               = i18n.Key("route.api.sign_in", "Sign In")
	L10nListOrders           = i18n.Key("route.api.list_orders", "List Orders")
	L10nPlaceOrder           = i18n.Key("route.api.place_order", "Place Order")
	L10nGetOrder             = i18n.Key("route.api.get_order", "Get Order")
	L10nGetOrderOrderId      = i18n.Key("route_param.api.get_order.orderId", "Order Id")
	L10nUploadReceipt        = i18n.Key("route.api.upload_receipt", "Upload Receipt")
	L10nUploadReceiptOrderId = i18n.Key("route_param.api.upload_receipt.orderId", "Order Id")
)

// l10n holds every label that has a path.
var l10n = map[string]i18n.TKer{
	"route.api.sign_in":                      L10nSignIn,
	"route.api.list_orders":                  L10nListOrders,
	"route.api.place_order":                  L10nPlaceOrder,
	"route.api.get_order":                    L10nGetOrder,
	"route_param.api.get_order.orderId":      L10nGetOrderOrderId,
	"route.api.upload_receipt":               L10nUploadReceipt,
	"route_param.api.upload_receipt.orderId": L10nUploadReceiptOrderId,
}

// L10n returns the label at path, or nil when there is none.
func L10n(path string) i18n.TKer { return l10n[path] }

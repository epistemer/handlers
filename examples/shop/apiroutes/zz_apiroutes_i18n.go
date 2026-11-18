// Code generated from labels by github.com/epistemer/i18n. DO NOT EDIT.

package apiroutes

import "github.com/epistemer/i18n"

var (
	L10nSignIn          = i18n.Key("route.api.sign_in", "Sign in")
	L10nSignInCode      = i18n.Key("route.api.sign_in_code", "Verify sign-in code")
	L10nGetMe           = i18n.Key("route.api.get_me", "Current session user")
	L10nListOrders      = i18n.Key("route.api.list_orders", "List orders")
	L10nGetOrder        = i18n.Key("route.api.get_order", "Get order")
	L10nGetOrderOrderId = i18n.Key("route_param.api.get_order.orderId", "Order Id")
)

// l10n holds every label that has a path.
var l10n = map[string]i18n.TKer{
	"route.api.sign_in":                 L10nSignIn,
	"route.api.sign_in_code":            L10nSignInCode,
	"route.api.get_me":                  L10nGetMe,
	"route.api.list_orders":             L10nListOrders,
	"route.api.get_order":               L10nGetOrder,
	"route_param.api.get_order.orderId": L10nGetOrderOrderId,
}

// L10n returns the label at path, or nil when there is none.
func L10n(path string) i18n.TKer { return l10n[path] }

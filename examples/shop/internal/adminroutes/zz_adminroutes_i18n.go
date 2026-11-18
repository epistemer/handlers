// Code generated from labels by github.com/epistemer/i18n. DO NOT EDIT.

package adminroutes

import "github.com/epistemer/i18n"

var (
	L10nSignInPage    = i18n.Key("route.sign_in", "Sign in")
	L10nSignInSubmit  = i18n.Key("route.sign_in.submit", "Submit sign-in")
	L10nSignInCode    = i18n.Key("route.sign_in.code", "Verify sign-in code")
	L10nDashboard     = i18n.Key("route.dashboard", "Dashboard")
	L10nProducts      = i18n.Key("route.products.list", "Products")
	L10nProduct       = i18n.Key("route.products.profile", "Product profile")
	L10nProductId     = i18n.Key("route_param.products.profile.id", "Product ID")
	L10nOrders        = i18n.Key("route.orders.list", "Orders")
	L10nOrder         = i18n.Key("route.orders.detail", "Order detail")
	L10nOrderId       = i18n.Key("route_param.orders.detail.id", "Order ID")
	L10nOrderCancel   = i18n.Key("route.orders.cancel", "Cancel order")
	L10nOrderCancelId = i18n.Key("route_param.orders.cancel.id", "Order ID")
	L10nSearch        = i18n.Key("route.search", "Search")
)

// l10n holds every label that has a path.
var l10n = map[string]i18n.TKer{
	"route.sign_in":                   L10nSignInPage,
	"route.sign_in.submit":            L10nSignInSubmit,
	"route.sign_in.code":              L10nSignInCode,
	"route.dashboard":                 L10nDashboard,
	"route.products.list":             L10nProducts,
	"route.products.profile":          L10nProduct,
	"route_param.products.profile.id": L10nProductId,
	"route.orders.list":               L10nOrders,
	"route.orders.detail":             L10nOrder,
	"route_param.orders.detail.id":    L10nOrderId,
	"route.orders.cancel":             L10nOrderCancel,
	"route_param.orders.cancel.id":    L10nOrderCancelId,
	"route.search":                    L10nSearch,
}

// L10n returns the label at path, or nil when there is none.
func L10n(path string) i18n.TKer { return l10n[path] }

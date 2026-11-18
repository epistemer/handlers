// Code generated from labels by github.com/epistemer/i18n. DO NOT EDIT.

package adminroutes

import "github.com/epistemer/i18n"

var (
	L10nDashboard     = i18n.Key("route.dashboard", "Dashboard")
	L10nOrders        = i18n.Key("route.orders.list", "List orders")
	L10nOrder         = i18n.Key("route.orders.detail", "Order detail")
	L10nOrderId       = i18n.Key("route_param.orders.detail.id", "Order ID")
	L10nOrderTok      = i18n.Key("route_param.orders.detail.tok", "Access token")
	L10nOrderCancel   = i18n.Key("route.orders.cancel", "Cancel order")
	L10nOrderCancelId = i18n.Key("route_param.orders.cancel.id", "Order ID")
	L10nFiles         = i18n.Key("route.files.serve", "Serve files")
	L10nFilesPath     = i18n.Key("route_param.files.serve.path", "File path")
	L10nFlags         = i18n.Key("route.flags.set", "Set flags")
	L10nFlagsOk       = i18n.Key("route_param.flags.set.ok", "Enabled")
	L10nFlagsN        = i18n.Key("route_param.flags.set.n", "Count")
)

// l10n holds every label that has a path.
var l10n = map[string]i18n.TKer{
	"route.dashboard":               L10nDashboard,
	"route.orders.list":             L10nOrders,
	"route.orders.detail":           L10nOrder,
	"route_param.orders.detail.id":  L10nOrderId,
	"route_param.orders.detail.tok": L10nOrderTok,
	"route.orders.cancel":           L10nOrderCancel,
	"route_param.orders.cancel.id":  L10nOrderCancelId,
	"route.files.serve":             L10nFiles,
	"route_param.files.serve.path":  L10nFilesPath,
	"route.flags.set":               L10nFlags,
	"route_param.flags.set.ok":      L10nFlagsOk,
	"route_param.flags.set.n":       L10nFlagsN,
}

// L10n returns the label at path, or nil when there is none.
func L10n(path string) i18n.TKer { return l10n[path] }

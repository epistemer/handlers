// Code generated from labels by github.com/epistemer/i18n. DO NOT EDIT.

package models

import "github.com/epistemer/i18n"

var (
	L10nOrderStatusPending   = i18n.Key("enum.order_status.pending", "pending")
	L10nOrderStatusPaid      = i18n.Key("enum.order_status.paid", "paid")
	L10nOrderStatusShipped   = i18n.Key("enum.order_status.shipped", "shipped")
	L10nOrderStatusCancelled = i18n.Key("enum.order_status.cancelled", "cancelled")
	L10nProduct              = i18n.Key("table.shop_product", "Product")
	L10nProduct_Id           = i18n.Key("table.shop_product.Product_Id", "ID")
	L10nProduct_Sku          = i18n.Key("table.shop_product.Product_Sku", "SKU")
	L10nProduct_Name         = i18n.Key("table.shop_product.Product_Name", "Name")
	L10nProduct_Price        = i18n.Key("table.shop_product.Product_Price", "Price")
	L10nOrder                = i18n.Key("table.shop_order", "Order")
	L10nOrder_Id             = i18n.Key("table.shop_order.Order_Id", "ID")
	L10nOrder_Status         = i18n.Key("table.shop_order.Order_Status", "Status")
	L10nOrder_Total          = i18n.Key("table.shop_order.Order_Total", "Total")
)

// l10n holds every label that has a path.
var l10n = map[string]i18n.TKer{
	"enum.order_status.pending":   L10nOrderStatusPending,
	"enum.order_status.paid":      L10nOrderStatusPaid,
	"enum.order_status.shipped":   L10nOrderStatusShipped,
	"enum.order_status.cancelled": L10nOrderStatusCancelled,
	"table.shop_product":          L10nProduct,
	"table.shop_product.id":       L10nProduct_Id,
	"table.shop_product.sku":      L10nProduct_Sku,
	"table.shop_product.name":     L10nProduct_Name,
	"table.shop_product.price":    L10nProduct_Price,
	"table.shop_order":            L10nOrder,
	"table.shop_order.id":         L10nOrder_Id,
	"table.shop_order.status":     L10nOrder_Status,
	"table.shop_order.total":      L10nOrder_Total,
}

// L10n returns the label at path, or nil when there is none.
func L10n(path string) i18n.TKer { return l10n[path] }

// Package ids holds typed path-parameter IDs shared by the shop domain and
// admin routes. Path parameter types live outside the admin handler package so
// the generated adminroutes package can import them without a cycle.
package ids

// ProductID identifies a product in URLs and storage.
type ProductID int64

// OrderID identifies an order in URLs and storage.
type OrderID int64

-- name: ListProducts :many
SELECT id, sku, name, price FROM shop_product ORDER BY id;

-- name: GetProduct :one
SELECT id, sku, name, price FROM shop_product WHERE id = $1;

-- name: ListOrders :many
SELECT id, status, total FROM shop_order ORDER BY id;

-- name: GetOrder :one
SELECT id, status, total FROM shop_order WHERE id = $1;

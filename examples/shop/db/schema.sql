CREATE TYPE order_status AS ENUM ('pending', 'paid', 'shipped', 'cancelled');

CREATE TABLE shop_product (
  id bigint PRIMARY KEY,
  sku text NOT NULL UNIQUE,
  name text NOT NULL,
  price text NOT NULL
);
COMMENT ON TABLE shop_product IS 'Product';
COMMENT ON COLUMN shop_product.id IS 'ID';
COMMENT ON COLUMN shop_product.sku IS 'SKU';
COMMENT ON COLUMN shop_product.name IS 'Name';
COMMENT ON COLUMN shop_product.price IS 'Price';

CREATE TABLE shop_order (
  id bigint PRIMARY KEY,
  status order_status NOT NULL,
  total text NOT NULL
);
COMMENT ON TABLE shop_order IS 'Order';
COMMENT ON COLUMN shop_order.id IS 'ID';
COMMENT ON COLUMN shop_order.status IS 'Status';
COMMENT ON COLUMN shop_order.total IS 'Total';

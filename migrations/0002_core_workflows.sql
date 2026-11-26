BEGIN;

ALTER TABLE orders ADD COLUMN checkout_digest bytea CHECK (octet_length(checkout_digest) = 32);
ALTER TABLE orders ADD COLUMN hold_until timestamptz;
ALTER TABLE service_circle_buildings ADD COLUMN active boolean NOT NULL DEFAULT true;
ALTER TABLE store_service_circles ADD COLUMN active boolean NOT NULL DEFAULT true;
ALTER TABLE stores ADD COLUMN delivery_fee_cents bigint NOT NULL DEFAULT 0 CHECK (delivery_fee_cents >= 0);
ALTER TABLE group_campaign_skus ADD COLUMN quota bigint CHECK (quota > 0);
ALTER TABLE group_campaign_skus ADD COLUMN paid_quantity bigint NOT NULL DEFAULT 0
    CHECK (paid_quantity >= 0 AND (quota IS NULL OR paid_quantity <= quota));

CREATE TABLE coupons (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores(id),
    code text NOT NULL CHECK (btrim(code) <> ''),
    discount_cents bigint NOT NULL CHECK (discount_cents > 0),
    minimum_cents bigint NOT NULL CHECK (minimum_cents >= 0),
    expires_at timestamptz NOT NULL,
    active boolean NOT NULL DEFAULT true,
    UNIQUE (store_id, code)
);

CREATE TABLE coupon_uses (
    coupon_id bigint NOT NULL REFERENCES coupons(id),
    buyer_id uuid NOT NULL,
    order_id bigint NOT NULL UNIQUE REFERENCES orders(id),
    PRIMARY KEY (coupon_id, buyer_id)
);

CREATE INDEX orders_expiring ON orders (hold_until, id)
    WHERE canceled_at IS NULL AND hold_until IS NOT NULL;

COMMIT;

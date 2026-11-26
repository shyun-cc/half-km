BEGIN;

CREATE TABLE communities (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE service_circles (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    community_id bigint NOT NULL REFERENCES communities (id),
    name text NOT NULL,
    UNIQUE (id, community_id)
);

CREATE TABLE buildings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    community_id bigint NOT NULL REFERENCES communities (id),
    name text NOT NULL,
    UNIQUE (id, community_id)
);

CREATE TABLE service_circle_buildings (
    circle_id bigint NOT NULL,
    building_id bigint NOT NULL,
    community_id bigint NOT NULL,
    PRIMARY KEY (circle_id, building_id),
    FOREIGN KEY (circle_id, community_id) REFERENCES service_circles (id, community_id),
    FOREIGN KEY (building_id, community_id) REFERENCES buildings (id, community_id)
);

CREATE TABLE stores (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE store_service_circles (
    store_id bigint NOT NULL REFERENCES stores (id),
    circle_id bigint NOT NULL REFERENCES service_circles (id),
    PRIMARY KEY (store_id, circle_id)
);

CREATE TABLE receiving_accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores (id),
    provider text NOT NULL,
    merchant_number text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    UNIQUE (provider, merchant_number),
    UNIQUE (id, store_id)
);

CREATE TABLE pickup_points (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    circle_id bigint NOT NULL REFERENCES service_circles (id),
    name text NOT NULL,
    address text NOT NULL,
    UNIQUE (id, circle_id)
);

CREATE TABLE leaders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    display_name text NOT NULL
);

CREATE TABLE pickup_point_leaders (
    pickup_point_id bigint NOT NULL REFERENCES pickup_points (id),
    leader_id bigint NOT NULL REFERENCES leaders (id),
    active boolean NOT NULL DEFAULT true,
    PRIMARY KEY (pickup_point_id, leader_id)
);

CREATE TABLE products (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores (id),
    name text NOT NULL,
    UNIQUE (id, store_id)
);

CREATE TABLE skus (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id bigint NOT NULL,
    product_id bigint NOT NULL,
    name text NOT NULL,
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    UNIQUE (id, store_id),
    FOREIGN KEY (product_id, store_id) REFERENCES products (id, store_id)
);

CREATE TABLE stock (
    sku_id bigint PRIMARY KEY REFERENCES skus (id),
    on_hand bigint NOT NULL CHECK (on_hand >= 0),
    reserved bigint NOT NULL DEFAULT 0 CHECK (reserved BETWEEN 0 AND on_hand)
);

CREATE TABLE group_campaigns (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id bigint NOT NULL,
    circle_id bigint NOT NULL,
    pickup_point_id bigint NOT NULL,
    leader_id bigint NOT NULL,
    commission_rate_bps integer NOT NULL CHECK (commission_rate_bps BETWEEN 0 AND 10000),
    cutoff_at timestamptz NOT NULL,
    UNIQUE (id, store_id),
    UNIQUE (id, store_id, circle_id),
    UNIQUE (id, pickup_point_id, leader_id),
    FOREIGN KEY (store_id, circle_id) REFERENCES store_service_circles (store_id, circle_id),
    FOREIGN KEY (pickup_point_id, circle_id) REFERENCES pickup_points (id, circle_id),
    FOREIGN KEY (pickup_point_id, leader_id) REFERENCES pickup_point_leaders (pickup_point_id, leader_id)
);

CREATE TABLE group_campaign_skus (
    campaign_id bigint NOT NULL,
    store_id bigint NOT NULL,
    sku_id bigint NOT NULL,
    PRIMARY KEY (campaign_id, sku_id),
    FOREIGN KEY (campaign_id, store_id) REFERENCES group_campaigns (id, store_id),
    FOREIGN KEY (sku_id, store_id) REFERENCES skus (id, store_id)
);

CREATE TABLE orders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    buyer_id uuid NOT NULL,
    checkout_key text NOT NULL,
    store_id bigint NOT NULL,
    circle_id bigint NOT NULL,
    receiving_account_id bigint NOT NULL,
    fulfillment text NOT NULL CHECK (fulfillment IN ('merchant_delivery', 'group_pickup')),
    building_id bigint,
    address_snapshot text,
    campaign_id bigint,
    commission_rate_bps integer CHECK (commission_rate_bps BETWEEN 0 AND 10000),
    delivery_fee_cents bigint NOT NULL DEFAULT 0 CHECK (delivery_fee_cents >= 0),
    payable_cents bigint NOT NULL CHECK (payable_cents >= delivery_fee_cents),
    created_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    canceled_at timestamptz,
    UNIQUE (buyer_id, checkout_key),
    UNIQUE (id, store_id, fulfillment),
    UNIQUE (id, campaign_id),
    UNIQUE (id, receiving_account_id, payable_cents),
    FOREIGN KEY (store_id, circle_id) REFERENCES store_service_circles (store_id, circle_id),
    FOREIGN KEY (receiving_account_id, store_id) REFERENCES receiving_accounts (id, store_id),
    FOREIGN KEY (circle_id, building_id) REFERENCES service_circle_buildings (circle_id, building_id),
    FOREIGN KEY (campaign_id, store_id, circle_id) REFERENCES group_campaigns (id, store_id, circle_id),
    CHECK (
        (fulfillment = 'merchant_delivery' AND building_id IS NOT NULL
            AND address_snapshot IS NOT NULL AND campaign_id IS NULL AND commission_rate_bps IS NULL)
        OR
        (fulfillment = 'group_pickup' AND building_id IS NULL AND address_snapshot IS NULL
            AND campaign_id IS NOT NULL AND commission_rate_bps IS NOT NULL AND delivery_fee_cents = 0)
    )
);

CREATE TABLE order_lines (
    order_id bigint NOT NULL,
    store_id bigint NOT NULL,
    fulfillment text NOT NULL,
    campaign_id bigint,
    sku_id bigint NOT NULL,
    sku_name_snapshot text NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    unit_price_cents bigint NOT NULL CHECK (unit_price_cents >= 0),
    discount_cents bigint NOT NULL DEFAULT 0 CHECK (
        discount_cents >= 0 AND discount_cents <= quantity::numeric * unit_price_cents
    ),
    PRIMARY KEY (order_id, sku_id),
    FOREIGN KEY (order_id, store_id, fulfillment) REFERENCES orders (id, store_id, fulfillment),
    FOREIGN KEY (order_id, campaign_id) REFERENCES orders (id, campaign_id),
    FOREIGN KEY (sku_id, store_id) REFERENCES skus (id, store_id),
    FOREIGN KEY (campaign_id, sku_id) REFERENCES group_campaign_skus (campaign_id, sku_id),
    CHECK ((fulfillment = 'group_pickup') = (campaign_id IS NOT NULL))
);

CREATE TABLE stock_reservations (
    order_id bigint NOT NULL,
    sku_id bigint NOT NULL REFERENCES stock (sku_id),
    quantity bigint NOT NULL CHECK (quantity > 0),
    state text NOT NULL CHECK (state IN ('held', 'consumed', 'released')),
    expires_at timestamptz NOT NULL,
    PRIMARY KEY (order_id, sku_id),
    FOREIGN KEY (order_id, sku_id) REFERENCES order_lines (order_id, sku_id)
);

CREATE TABLE payments (
    order_id bigint PRIMARY KEY,
    receiving_account_id bigint NOT NULL,
    provider_transaction_id text NOT NULL CHECK (provider_transaction_id <> ''),
    amount_cents bigint NOT NULL CHECK (amount_cents >= 0),
    paid_at timestamptz NOT NULL,
    disposition text NOT NULL CHECK (disposition IN ('accepted', 'refund_required')),
    recorded_at timestamptz NOT NULL CHECK (recorded_at >= paid_at),
    UNIQUE (receiving_account_id, provider_transaction_id),
    UNIQUE (order_id, receiving_account_id),
    UNIQUE (order_id, disposition),
    FOREIGN KEY (order_id, receiving_account_id, amount_cents)
        REFERENCES orders (id, receiving_account_id, payable_cents)
);

CREATE TABLE refunds (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id bigint NOT NULL,
    receiving_account_id bigint NOT NULL,
    request_key text NOT NULL,
    provider_refund_id text,
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    state text NOT NULL CHECK (state IN ('pending', 'succeeded', 'failed')),
    UNIQUE (order_id, request_key),
    UNIQUE (receiving_account_id, provider_refund_id),
    FOREIGN KEY (order_id, receiving_account_id) REFERENCES payments (order_id, receiving_account_id)
);

CREATE TABLE merchant_deliveries (
    order_id bigint PRIMARY KEY,
    payment_disposition text NOT NULL DEFAULT 'accepted' CHECK (payment_disposition = 'accepted'),
    store_id bigint NOT NULL,
    fulfillment text NOT NULL DEFAULT 'merchant_delivery' CHECK (fulfillment = 'merchant_delivery'),
    state text NOT NULL CHECK (state IN ('accepted', 'out_for_delivery', 'delivered')),
    updated_at timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (order_id, store_id, fulfillment) REFERENCES orders (id, store_id, fulfillment),
    FOREIGN KEY (order_id, payment_disposition) REFERENCES payments (order_id, disposition)
);

CREATE TABLE group_batches (
    campaign_id bigint PRIMARY KEY,
    pickup_point_id bigint NOT NULL,
    leader_id bigint NOT NULL,
    closed_at timestamptz NOT NULL,
    arrived_at timestamptz CHECK (arrived_at >= closed_at),
    UNIQUE (campaign_id, pickup_point_id, leader_id),
    FOREIGN KEY (campaign_id, pickup_point_id, leader_id)
        REFERENCES group_campaigns (id, pickup_point_id, leader_id)
);

CREATE TABLE group_batch_orders (
    campaign_id bigint NOT NULL REFERENCES group_batches (campaign_id),
    order_id bigint NOT NULL UNIQUE,
    payment_disposition text NOT NULL DEFAULT 'accepted' CHECK (payment_disposition = 'accepted'),
    PRIMARY KEY (campaign_id, order_id),
    FOREIGN KEY (order_id, payment_disposition) REFERENCES payments (order_id, disposition),
    FOREIGN KEY (order_id, campaign_id) REFERENCES orders (id, campaign_id)
);

CREATE TABLE group_sorting_lines (
    campaign_id bigint NOT NULL REFERENCES group_batches (campaign_id),
    sku_id bigint NOT NULL,
    quantity bigint NOT NULL CHECK (quantity > 0),
    PRIMARY KEY (campaign_id, sku_id),
    FOREIGN KEY (campaign_id, sku_id) REFERENCES group_campaign_skus (campaign_id, sku_id)
);

CREATE TABLE pickup_redemptions (
    order_id bigint PRIMARY KEY,
    campaign_id bigint NOT NULL,
    pickup_point_id bigint NOT NULL,
    leader_id bigint NOT NULL,
    code_digest bytea NOT NULL CHECK (octet_length(code_digest) = 32),
    event_id uuid UNIQUE,
    redeemed_at timestamptz,
    UNIQUE (order_id, campaign_id, leader_id),
    FOREIGN KEY (campaign_id, order_id) REFERENCES group_batch_orders (campaign_id, order_id),
    FOREIGN KEY (campaign_id, pickup_point_id, leader_id)
        REFERENCES group_batches (campaign_id, pickup_point_id, leader_id),
    CHECK ((event_id IS NULL) = (redeemed_at IS NULL))
);

CREATE TABLE commission_statements (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    store_id bigint NOT NULL REFERENCES stores (id),
    leader_id bigint NOT NULL REFERENCES leaders (id),
    period_start date NOT NULL,
    period_end date NOT NULL CHECK (period_end > period_start),
    total_cents bigint NOT NULL,
    offline_payment_reference text,
    offline_paid_at timestamptz,
    UNIQUE (store_id, leader_id, period_start, period_end),
    UNIQUE (id, store_id, leader_id),
    UNIQUE (store_id, offline_payment_reference),
    CHECK ((offline_payment_reference IS NULL) = (offline_paid_at IS NULL)),
    CHECK (offline_payment_reference IS NULL OR btrim(offline_payment_reference) <> '')
);

CREATE TABLE commission_entries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id bigint NOT NULL,
    campaign_id bigint NOT NULL,
    store_id bigint NOT NULL,
    leader_id bigint NOT NULL,
    statement_id bigint,
    event_key text NOT NULL CHECK (event_key <> ''),
    entry_kind text NOT NULL CHECK (entry_kind IN ('earned', 'refund_adjustment')),
    amount_cents bigint NOT NULL,
    eligible_at timestamptz NOT NULL,
    UNIQUE (order_id, event_key),
    FOREIGN KEY (order_id, campaign_id, leader_id)
        REFERENCES pickup_redemptions (order_id, campaign_id, leader_id),
    FOREIGN KEY (campaign_id, store_id) REFERENCES group_campaigns (id, store_id),
    FOREIGN KEY (statement_id, store_id, leader_id)
        REFERENCES commission_statements (id, store_id, leader_id),
    CHECK ((entry_kind = 'earned' AND amount_cents >= 0)
        OR (entry_kind = 'refund_adjustment' AND amount_cents <= 0))
);

CREATE UNIQUE INDEX commission_entries_one_earned ON commission_entries (order_id) WHERE entry_kind = 'earned';
CREATE INDEX commission_entries_unassigned ON commission_entries (store_id, leader_id, eligible_at)
    WHERE statement_id IS NULL;
CREATE INDEX commission_entries_statement ON commission_entries (statement_id);
CREATE INDEX group_campaigns_cutoff ON group_campaigns (cutoff_at);
CREATE INDEX orders_campaign ON orders (campaign_id);
CREATE INDEX orders_store_created ON orders (store_id, created_at DESC);
CREATE INDEX stock_reservations_expiring ON stock_reservations (expires_at) WHERE state = 'held';
CREATE INDEX store_service_circles_circle ON store_service_circles (circle_id, store_id);
CREATE INDEX service_circle_buildings_building ON service_circle_buildings (building_id, circle_id);
CREATE INDEX pickup_point_leaders_leader ON pickup_point_leaders (leader_id, pickup_point_id);
CREATE INDEX skus_product ON skus (product_id, store_id);

COMMIT;

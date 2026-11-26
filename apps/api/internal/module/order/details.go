package order

import (
	"context"
	"database/sql"
	"encoding/json"
)

func Details(ctx context.Context, tx *sql.Tx, id int64) (json.RawMessage, error) {
	var document json.RawMessage
	err := tx.QueryRowContext(ctx, `SELECT jsonb_build_object(
		'id',o.id,'store_id',o.store_id,'circle_id',o.circle_id,'fulfillment',o.fulfillment,
		'building_id',o.building_id,'address_snapshot',o.address_snapshot,'campaign_id',o.campaign_id,
		'pickup_point_id',b.pickup_point_id,'arrived_at',b.arrived_at,
		'commission_rate_bps',o.commission_rate_bps,'delivery_fee_cents',o.delivery_fee_cents,
		'payable_cents',o.payable_cents,'created_at',o.created_at,'hold_until',o.hold_until,'canceled_at',o.canceled_at,
		'receiving_account',jsonb_build_object('id',a.id,'provider',a.provider,'merchant_number',a.merchant_number),
		'payment_disposition',p.disposition,'delivery_state',d.state,'redeemed_at',r.redeemed_at,
		'refunded_cents',COALESCE((SELECT SUM(amount_cents) FROM refunds WHERE order_id=o.id AND state='succeeded'),0),
		'items',COALESCE((SELECT jsonb_agg(to_jsonb(l)-'order_id'-'store_id'-'campaign_id'-'fulfillment' ORDER BY sku_id)
			FROM order_lines l WHERE order_id=o.id),'[]'::jsonb)
	) FROM orders o JOIN receiving_accounts a ON a.id=o.receiving_account_id
	LEFT JOIN payments p ON p.order_id=o.id LEFT JOIN merchant_deliveries d ON d.order_id=o.id
	LEFT JOIN group_batches b ON b.campaign_id=o.campaign_id
	LEFT JOIN pickup_redemptions r ON r.order_id=o.id WHERE o.id=$1`, id).Scan(&document)
	return document, err
}

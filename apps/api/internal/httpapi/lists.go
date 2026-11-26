package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

type listScope func(request) (any, error)

func pathID(name string) listScope {
	return func(r request) (any, error) { return r.id(name) }
}

func ownedStore(r request) (any, error) { return r.storeID() }

func list(api API, server *ghttp.Server, route string, roles []string, scope listScope, query string) {
	bind(api, server, "GET:/v1/"+route, roles, func(ctx context.Context, tx *sql.Tx, r request, _ empty) ([]json.RawMessage, error) {
		var subject any = int64(0)
		if scope != nil {
			var err error
			subject, err = scope(r)
			if err != nil {
				return nil, err
			}
		}
		after := r.raw.GetQuery("after", "0").String()
		cursor, err := strconv.ParseInt(after, 10, 64)
		if err != nil || cursor < 0 {
			return nil, dbtx.ErrInvalid
		}
		rows, err := tx.QueryContext(ctx, `SELECT to_jsonb(result) FROM (`+query+`) result WHERE id>$2 ORDER BY id LIMIT 100`, subject, cursor)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		result := make([]json.RawMessage, 0)
		for rows.Next() {
			var entry json.RawMessage
			if err := rows.Scan(&entry); err != nil {
				return nil, err
			}
			result = append(result, entry)
		}
		return result, rows.Err()
	})
}

func (api API) bindLists(server *ghttp.Server) {
	list(api, server, "communities", users, nil, `SELECT id,name FROM communities WHERE $1::bigint=0`)
	list(api, server, "communities/:community/circles", users, pathID("community"), `SELECT id,name,community_id FROM service_circles WHERE community_id=$1`)
	list(api, server, "communities/:community/buildings", operator, pathID("community"), `SELECT id,name,community_id FROM buildings WHERE community_id=$1`)
	list(api, server, "circles/:circle/buildings", users, pathID("circle"), `SELECT b.id,b.name,a.active FROM service_circle_buildings a JOIN buildings b ON b.id=a.building_id WHERE a.circle_id=$1 AND a.active`)
	list(api, server, "circles/:circle/stores", users, pathID("circle"), `SELECT s.id,s.name,s.delivery_fee_cents FROM store_service_circles c JOIN stores s ON s.id=c.store_id WHERE c.circle_id=$1 AND c.active`)
	list(api, server, "circles/:circle/pickup-points", users, pathID("circle"), `SELECT id,name,address FROM pickup_points WHERE circle_id=$1`)
	list(api, server, "stores", operator, nil, `SELECT id,name,delivery_fee_cents FROM stores WHERE $1::bigint=0`)
	list(api, server, "stores/:store/circles", storeAdmin, ownedStore, `SELECT c.id,c.name,s.active
		FROM service_circles c JOIN store_service_circles s ON c.id=s.circle_id WHERE s.store_id=$1`)
	list(api, server, "leaders", operator, nil, `SELECT id,display_name FROM leaders WHERE $1::bigint=0`)
	list(api, server, "pickup-points/:point/leaders", storeAdmin, pathID("point"), `SELECT l.id,l.display_name,p.active FROM pickup_point_leaders p JOIN leaders l ON l.id=p.leader_id WHERE p.pickup_point_id=$1`)
	list(api, server, "stores/:store/products", users, pathID("store"), `SELECT id,name FROM products WHERE store_id=$1`)
	list(api, server, "stores/:store/skus", users, pathID("store"), `SELECT k.id,k.product_id,k.name,k.unit_price_cents,
		COALESCE(s.on_hand-s.reserved,0) AS available FROM skus k LEFT JOIN stock s ON s.sku_id=k.id WHERE k.store_id=$1`)
	list(api, server, "stores/:store/stock", storeAdmin, ownedStore, `SELECT k.id,COALESCE(s.on_hand,0) AS on_hand,COALESCE(s.reserved,0) AS reserved
		FROM skus k LEFT JOIN stock s ON s.sku_id=k.id WHERE k.store_id=$1`)
	list(api, server, "stores/:store/coupons", storeAdmin, ownedStore, `SELECT id,code,discount_cents,minimum_cents,expires_at,active FROM coupons WHERE store_id=$1`)
	list(api, server, "stores/:store/receiving-accounts", storeAdmin, ownedStore, `SELECT id,provider,merchant_number,active FROM receiving_accounts WHERE store_id=$1`)
	list(api, server, "stores/:store/checkout-accounts", users, pathID("store"), `SELECT id,provider FROM receiving_accounts WHERE store_id=$1 AND active`)
	list(api, server, "circles/:circle/campaigns", users, pathID("circle"), `SELECT c.id,c.store_id,c.pickup_point_id,c.cutoff_at,
		(SELECT jsonb_agg(jsonb_build_object('sku_id',s.sku_id,'remaining',s.quota-s.paid_quantity) ORDER BY s.sku_id)
			FROM group_campaign_skus s WHERE s.campaign_id=c.id) AS offers
		FROM group_campaigns c JOIN store_service_circles sc USING (store_id,circle_id)
		WHERE c.circle_id=$1 AND sc.active AND c.cutoff_at>clock_timestamp()`)
	list(api, server, "stores/:store/campaigns", merchantRole, ownedStore, `SELECT c.id,c.circle_id,c.pickup_point_id,c.leader_id,c.cutoff_at,
		c.commission_rate_bps,b.closed_at,b.arrived_at FROM group_campaigns c LEFT JOIN group_batches b ON b.campaign_id=c.id WHERE c.store_id=$1`)
	list(api, server, "orders", buyerRole, func(r request) (any, error) { return r.user.Buyer, nil },
		`SELECT o.id,o.store_id,o.fulfillment,o.payable_cents,o.created_at,o.canceled_at,p.disposition
		FROM orders o LEFT JOIN payments p ON p.order_id=o.id WHERE o.buyer_id=$1`)
	list(api, server, "stores/:store/orders", merchantRole, ownedStore,
		`SELECT o.id,o.fulfillment,o.payable_cents,o.created_at,o.canceled_at,p.disposition
		FROM orders o LEFT JOIN payments p ON p.order_id=o.id WHERE o.store_id=$1`)
	list(api, server, "stores/:store/refund-required", []string{"payments"}, ownedStore, `SELECT o.id,p.receiving_account_id,
		p.provider_transaction_id,p.amount_cents-COALESCE(r.total,0) AS outstanding_cents
		FROM orders o JOIN payments p ON p.order_id=o.id
		LEFT JOIN LATERAL (SELECT SUM(amount_cents) AS total FROM refunds WHERE order_id=o.id AND state='succeeded') r ON true
		WHERE o.store_id=$1 AND p.disposition='refund_required' AND p.amount_cents>COALESCE(r.total,0)`)
	list(api, server, "stores/:store/commission-statements", merchantRole, ownedStore,
		`SELECT id,leader_id,period_start,period_end,total_cents,offline_payment_reference,offline_paid_at
		FROM commission_statements WHERE store_id=$1`)
	list(api, server, "stores/:store/commission-entries", merchantRole, ownedStore,
		`SELECT id,order_id,leader_id,statement_id,entry_kind,amount_cents,eligible_at FROM commission_entries WHERE store_id=$1`)
}

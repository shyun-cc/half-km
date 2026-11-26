package groupbuy

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/inventory"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
)

type Offer struct {
	SKU   int64 `json:"sku_id"`
	Quota int64 `json:"quota"`
}

type OpenInput struct {
	Circle int64     `json:"circle_id"`
	Point  int64     `json:"pickup_point_id"`
	Leader int64     `json:"leader_id"`
	Rate   int64     `json:"commission_rate_bps"`
	Cutoff time.Time `json:"cutoff_at"`
	Offers []Offer   `json:"offers"`
}

func Open(ctx context.Context, tx *sql.Tx, store int64, input OpenInput) (int64, error) {
	if len(input.Offers) == 0 || len(input.Offers) > 100 || input.Rate < 0 || input.Rate > 10000 {
		return 0, dbtx.ErrInvalid
	}
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO group_campaigns (store_id,circle_id,pickup_point_id,leader_id,commission_rate_bps,cutoff_at)
		SELECT $1,$2,$3,$4,$5,$6 WHERE $6>clock_timestamp() AND EXISTS (
			SELECT 1 FROM store_service_circles WHERE store_id=$1 AND circle_id=$2 AND active FOR SHARE
		) AND EXISTS (
			SELECT 1 FROM pickup_point_leaders WHERE pickup_point_id=$3 AND leader_id=$4 AND active FOR SHARE
		) RETURNING id`, store, input.Circle, input.Point, input.Leader, input.Rate, input.Cutoff).Scan(&id)
	if err != nil {
		return 0, err
	}
	for _, offer := range input.Offers {
		if offer.SKU <= 0 || offer.Quota <= 0 {
			return 0, dbtx.ErrInvalid
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO group_campaign_skus (campaign_id,store_id,sku_id,quota)
			VALUES ($1,$2,$3,$4)`, id, store, offer.SKU, offer.Quota); err != nil {
			return 0, err
		}
	}
	return id, nil
}

func Claim(ctx context.Context, tx *sql.Tx, o order.Order) error {
	items, err := order.Items(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return dbtx.ErrConflict
	}
	for _, item := range items {
		if err := dbtx.Changed(tx.ExecContext(ctx, `UPDATE group_campaign_skus SET paid_quantity=paid_quantity+$3
			WHERE campaign_id=$1 AND sku_id=$2 AND (quota IS NULL OR quota-paid_quantity >= $3)`, o.Campaign, item.SKU, item.Quantity)); err != nil {
			return err
		}
		if err := inventory.Take(ctx, tx, o.ID, item.SKU, item.Quantity, o.Cutoff); err != nil {
			return err
		}
	}
	return nil
}

func ReleaseBeforeClose(ctx context.Context, tx *sql.Tx, o order.Order) error {
	var frozen bool
	if err := tx.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM group_batches WHERE campaign_id=$1)", o.Campaign).Scan(&frozen); err != nil || frozen {
		return err
	}
	items, err := order.Items(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	for _, item := range items {
		quantity, err := inventory.ReleaseConsumed(ctx, tx, o.ID, item.SKU)
		if err != nil {
			return err
		}
		if quantity == 0 {
			continue
		}
		if err := dbtx.Changed(tx.ExecContext(ctx, `UPDATE group_campaign_skus SET paid_quantity=CASE WHEN quota IS NULL THEN GREATEST(0,paid_quantity-$3) ELSE paid_quantity-$3 END
			WHERE campaign_id=$1 AND sku_id=$2`, o.Campaign, item.SKU, quantity)); err != nil {
			return err
		}
	}
	return nil
}

// Close uses the same row lock as payment and builds the batch exactly once.
func Close(ctx context.Context, tx *sql.Tx, id, store int64) (json.RawMessage, error) {
	var cutoff time.Time
	var point, leader int64
	err := tx.QueryRowContext(ctx, `SELECT cutoff_at,pickup_point_id,leader_id FROM group_campaigns
		WHERE id=$1 AND ($2=0 OR store_id=$2) FOR NO KEY UPDATE`, id, store).Scan(&cutoff, &point, &leader)
	if err != nil {
		return nil, err
	}
	batch, err := ReadBatch(ctx, tx, id)
	if !errors.Is(err, sql.ErrNoRows) {
		return batch, err
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return nil, err
	}
	if now.Before(cutoff) {
		return nil, dbtx.ErrConflict
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO group_batches (campaign_id,pickup_point_id,leader_id,closed_at)
		VALUES ($1,$2,$3,$4)`, id, point, leader, now); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO group_batch_orders (campaign_id,order_id)
		SELECT $1,o.id FROM orders o JOIN payments p ON p.order_id=o.id WHERE o.campaign_id=$1
		AND o.canceled_at IS NULL AND p.disposition='accepted' AND p.recorded_at<$2`, id, cutoff); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO group_sorting_lines (campaign_id,sku_id,quantity)
		SELECT $1,l.sku_id,SUM(l.quantity) FROM group_batch_orders b JOIN order_lines l ON l.order_id=b.order_id
		WHERE b.campaign_id=$1 GROUP BY l.sku_id`, id); err != nil {
		return nil, err
	}
	return ReadBatch(ctx, tx, id)
}

func ReadBatch(ctx context.Context, tx *sql.Tx, id int64) (json.RawMessage, error) {
	var result json.RawMessage
	err := tx.QueryRowContext(ctx, `SELECT to_jsonb(b) || jsonb_build_object(
		'orders',COALESCE((SELECT jsonb_agg(order_id ORDER BY order_id) FROM group_batch_orders WHERE campaign_id=b.campaign_id),'[]'::jsonb),
		'order_statuses',COALESCE((SELECT jsonb_agg(jsonb_build_object('order_id',bo.order_id,'canceled_at',o.canceled_at,
			'redeemed_at',r.redeemed_at) ORDER BY bo.order_id)
			FROM group_batch_orders bo JOIN orders o ON o.id=bo.order_id LEFT JOIN pickup_redemptions r ON r.order_id=bo.order_id
			WHERE bo.campaign_id=b.campaign_id),'[]'::jsonb),
		'sorting_lines',COALESCE((SELECT jsonb_agg(jsonb_build_object('sku_id',l.sku_id,'name',s.name,'quantity',l.quantity) ORDER BY l.sku_id)
			FROM group_sorting_lines l JOIN skus s ON s.id=l.sku_id WHERE l.campaign_id=b.campaign_id),'[]'::jsonb))
		FROM group_batches b WHERE campaign_id=$1`, id).Scan(&result)
	return result, err
}

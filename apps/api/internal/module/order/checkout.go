package order

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/inventory"
	"github.com/shyun-cc/half-km/apps/api/internal/module/marketing"
)

type Item struct {
	SKU      int64 `json:"sku_id"`
	Quantity int64 `json:"quantity"`
}

type Checkout struct {
	Key      string `json:"checkout_key"`
	Store    int64  `json:"store_id"`
	Circle   int64  `json:"circle_id"`
	Account  int64  `json:"receiving_account_id"`
	Building int64  `json:"building_id"`
	Address  string `json:"address"`
	Campaign int64  `json:"campaign_id"`
	Coupon   string `json:"coupon_code"`
	Items    []Item `json:"items"`
}

type Order struct {
	ID         int64        `json:"id"`
	Buyer      string       `json:"buyer_id"`
	Store      int64        `json:"store_id"`
	Account    int64        `json:"receiving_account_id"`
	Campaign   int64        `json:"campaign_id,omitempty"`
	Rate       int64        `json:"commission_rate_bps"`
	Fee        int64        `json:"delivery_fee_cents"`
	Payable    int64        `json:"payable_cents"`
	HoldUntil  sql.NullTime `json:"-"`
	CanceledAt sql.NullTime `json:"-"`
	Cutoff     time.Time    `json:"-"`
}

func Get(ctx context.Context, tx *sql.Tx, id int64, lock bool) (Order, error) {
	var o Order
	// Campaign membership never changes. Serialize all group transitions before locking the order.
	if lock {
		var campaign sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT campaign_id FROM orders WHERE id=$1", id).Scan(&campaign); err != nil {
			return o, err
		}
		if campaign.Valid {
			if err := tx.QueryRowContext(ctx, "SELECT cutoff_at FROM group_campaigns WHERE id=$1 FOR NO KEY UPDATE", campaign.Int64).Scan(&o.Cutoff); err != nil {
				return o, err
			}
		}
	}
	query := `SELECT id,buyer_id::text,store_id,receiving_account_id,COALESCE(campaign_id,0),
		COALESCE(commission_rate_bps,0),delivery_fee_cents,payable_cents,hold_until,canceled_at FROM orders WHERE id=$1`
	if lock {
		query += " FOR NO KEY UPDATE"
	}
	err := tx.QueryRowContext(ctx, query, id).Scan(&o.ID, &o.Buyer, &o.Store, &o.Account, &o.Campaign, &o.Rate, &o.Fee, &o.Payable, &o.HoldUntil, &o.CanceledAt)
	return o, err
}

type checkoutLine struct {
	Item
	Name     string
	Price    int64
	Discount int64
}

func Place(ctx context.Context, tx *sql.Tx, buyer string, req Checkout, holdDuration time.Duration) (Order, error) {
	req.Key = strings.TrimSpace(req.Key)
	req.Address = strings.TrimSpace(req.Address)
	req.Coupon = strings.TrimSpace(req.Coupon)
	if _, err := uuid.Parse(buyer); err != nil || req.Key == "" || len(req.Key) > 128 || len(req.Items) == 0 || len(req.Items) > 100 ||
		req.Store <= 0 || req.Circle <= 0 || req.Account <= 0 || req.Campaign < 0 {
		return Order{}, dbtx.ErrInvalid
	}
	mode := MerchantDelivery
	if req.Campaign > 0 {
		mode = GroupPickup
		if req.Building != 0 || req.Address != "" {
			return Order{}, dbtx.ErrInvalid
		}
	} else if req.Building <= 0 || req.Address == "" || len(req.Address) > 512 {
		return Order{}, dbtx.ErrInvalid
	}
	req.Items = slices.Clone(req.Items)
	slices.SortFunc(req.Items, func(a, b Item) int { return cmp.Compare(a.SKU, b.SKU) })
	for i, item := range req.Items {
		if item.SKU <= 0 || item.Quantity <= 0 || (i > 0 && item.SKU == req.Items[i-1].SKU) {
			return Order{}, dbtx.ErrInvalid
		}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Order{}, err
	}
	digest := sha256.Sum256(body)
	if err := dbtx.Lock(ctx, tx, "checkout/"+buyer+"/"+req.Key); err != nil {
		return Order{}, err
	}
	var id int64
	var existing []byte
	err = tx.QueryRowContext(ctx, "SELECT id,checkout_digest FROM orders WHERE buyer_id=$1 AND checkout_key=$2", buyer, req.Key).Scan(&id, &existing)
	if err == nil {
		if !bytes.Equal(existing, digest[:]) {
			return Order{}, dbtx.ErrConflict
		}
		return Get(ctx, tx, id, false)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Order{}, err
	}

	var cutoff time.Time
	var fee, rate int64
	if mode == GroupPickup {
		err = tx.QueryRowContext(ctx, `SELECT c.cutoff_at,c.commission_rate_bps FROM group_campaigns c
			JOIN store_service_circles sc USING (store_id,circle_id)
			WHERE c.id=$1 AND c.store_id=$2 AND c.circle_id=$3 AND sc.active FOR NO KEY UPDATE OF c FOR SHARE OF sc`, req.Campaign, req.Store, req.Circle).Scan(&cutoff, &rate)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT s.delivery_fee_cents FROM stores s
			JOIN store_service_circles sc ON sc.store_id=s.id
			JOIN service_circle_buildings b ON b.circle_id=sc.circle_id
			WHERE s.id=$1 AND sc.circle_id=$2 AND b.building_id=$3 AND b.active AND sc.active FOR SHARE OF s, sc, b`, req.Store, req.Circle, req.Building).Scan(&fee)
	}
	if err != nil {
		return Order{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT id FROM receiving_accounts WHERE id=$1 AND store_id=$2 AND active FOR SHARE`, req.Account, req.Store).Scan(&id); err != nil {
		return Order{}, err
	}

	lines := make([]checkoutLine, 0, len(req.Items))
	var subtotal int64
	for _, item := range req.Items {
		line := checkoutLine{Item: item}
		query := `SELECT s.name,s.unit_price_cents FROM skus s WHERE s.id=$1 AND s.store_id=$2
			AND ($3=0 OR EXISTS (SELECT 1 FROM group_campaign_skus WHERE campaign_id=$3 AND sku_id=s.id))
			FOR SHARE OF s`
		if err := tx.QueryRowContext(ctx, query, item.SKU, req.Store, req.Campaign).Scan(&line.Name, &line.Price); err != nil {
			return Order{}, err
		}
		if line.Price > math.MaxInt64/item.Quantity || line.Price*item.Quantity > math.MaxInt64-subtotal {
			return Order{}, dbtx.ErrInvalid
		}
		subtotal += line.Price * item.Quantity
		if mode == MerchantDelivery {
			if err := inventory.Hold(ctx, tx, item.SKU, item.Quantity); err != nil {
				return Order{}, err
			}
		}
		lines = append(lines, line)
	}

	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return Order{}, err
	}
	expires := now.Add(holdDuration)
	if mode == GroupPickup {
		if !now.Before(cutoff) {
			return Order{}, dbtx.ErrConflict
		}
		expires = cutoff
	}
	var coupon, discount int64
	if req.Coupon != "" {
		coupon, discount, err = marketing.Coupon(ctx, tx, req.Store, req.Coupon, subtotal, now)
		if err != nil {
			return Order{}, err
		}
	}
	snapshot := Snapshot{ID: req.Key, StoreID: strconv.FormatInt(req.Store, 10), ReceivingAccountID: strconv.FormatInt(req.Account, 10), Fulfillment: mode, DeliveryFeeCents: fee}
	for i := range lines {
		line := &lines[i]
		line.Discount = min(discount, line.Price*line.Quantity)
		discount -= line.Discount
		snapshot.Lines = append(snapshot.Lines, Line{SKU: strconv.FormatInt(line.SKU, 10), Quantity: line.Quantity, UnitPriceCents: line.Price, DiscountCents: line.Discount})
	}
	payable, err := snapshot.PayableCents()
	if err != nil {
		return Order{}, dbtx.ErrInvalid
	}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO orders (buyer_id,checkout_key,checkout_digest,store_id,circle_id,receiving_account_id,fulfillment,
			building_id,address_snapshot,campaign_id,commission_rate_bps,delivery_fee_cents,payable_cents,created_at,hold_until)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8::bigint,0),NULLIF($9,''),NULLIF($10::bigint,0),
			CASE WHEN $10=0 THEN NULL ELSE $11::integer END,$12,$13,$14,$15) RETURNING id`,
		buyer, req.Key, digest[:], req.Store, req.Circle, req.Account, mode, req.Building, req.Address, req.Campaign, rate, fee, payable, now, expires).Scan(&id)
	if err != nil {
		return Order{}, err
	}
	for _, line := range lines {
		if _, err := tx.ExecContext(ctx, `INSERT INTO order_lines (order_id,store_id,fulfillment,campaign_id,sku_id,
			sku_name_snapshot,quantity,unit_price_cents,discount_cents) VALUES ($1,$2,$3,NULLIF($4::bigint,0),$5,$6,$7,$8,$9)`,
			id, req.Store, mode, req.Campaign, line.SKU, line.Name, line.Quantity, line.Price, line.Discount); err != nil {
			return Order{}, err
		}
		if mode == MerchantDelivery {
			if err := inventory.RecordHold(ctx, tx, id, line.SKU, line.Quantity, expires); err != nil {
				return Order{}, err
			}
		}
	}
	if coupon != 0 {
		if err := marketing.Use(ctx, tx, coupon, id, buyer); err != nil {
			return Order{}, err
		}
	}
	return Get(ctx, tx, id, false)
}

func Items(ctx context.Context, tx *sql.Tx, id int64) ([]Item, error) {
	rows, err := tx.QueryContext(ctx, "SELECT sku_id,quantity FROM order_lines WHERE order_id=$1 ORDER BY sku_id", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.SKU, &item.Quantity); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func Cancel(ctx context.Context, tx *sql.Tx, o Order, now time.Time) error {
	if o.CanceledAt.Valid {
		return nil
	}
	if err := inventory.ResolveHolds(ctx, tx, o.ID, false); err != nil {
		return err
	}
	if err := marketing.Release(ctx, tx, o.ID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "UPDATE orders SET canceled_at=$2 WHERE id=$1", o.ID, now)
	return err
}

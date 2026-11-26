package delivery

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/inventory"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
)

func Advance(ctx context.Context, tx *sql.Tx, o order.Order, next string) (string, error) {
	if o.Campaign != 0 || o.CanceledAt.Valid {
		return "", dbtx.ErrConflict
	}
	prior := map[string]string{"accepted": "", "out_for_delivery": "accepted", "delivered": "out_for_delivery"}
	previous, ok := prior[next]
	if !ok {
		return "", dbtx.ErrInvalid
	}
	var current string
	err := tx.QueryRowContext(ctx, "SELECT state FROM merchant_deliveries WHERE order_id=$1", o.ID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if current == next {
		return current, nil
	}
	if current != previous {
		return "", dbtx.ErrConflict
	}
	if next == "accepted" {
		err = dbtx.Changed(tx.ExecContext(ctx, `INSERT INTO merchant_deliveries (order_id,store_id,state)
			SELECT order_id,$2,'accepted' FROM payments WHERE order_id=$1 AND disposition='accepted'`, o.ID, o.Store))
	} else {
		err = dbtx.Changed(tx.ExecContext(ctx, `UPDATE merchant_deliveries SET state=$2,updated_at=clock_timestamp() WHERE order_id=$1`, o.ID, next))
	}
	return next, err
}

func Arrive(ctx context.Context, tx *sql.Tx, campaign, store int64) (time.Time, error) {
	var at time.Time
	err := tx.QueryRowContext(ctx, `UPDATE group_batches b SET arrived_at=COALESCE(b.arrived_at,clock_timestamp())
		FROM group_campaigns c WHERE b.campaign_id=$1 AND c.id=b.campaign_id AND c.store_id=$2 RETURNING b.arrived_at`,
		campaign, store).Scan(&at)
	return at, err
}

func Code(ctx context.Context, tx *sql.Tx, o order.Order, key []byte) (string, error) {
	if o.Campaign == 0 || o.CanceledAt.Valid {
		return "", dbtx.ErrConflict
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("half-km/pickup/" + strconv.FormatInt(o.ID, 10) + "/" + o.Buyer))
	code := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	digest := sha256.Sum256([]byte(code))
	var stored []byte
	err := tx.QueryRowContext(ctx, `INSERT INTO pickup_redemptions (order_id,campaign_id,pickup_point_id,leader_id,code_digest)
		SELECT bo.order_id,b.campaign_id,b.pickup_point_id,b.leader_id,$2
		FROM group_batch_orders bo JOIN group_batches b USING (campaign_id) WHERE bo.order_id=$1
		ON CONFLICT (order_id) DO UPDATE SET code_digest=pickup_redemptions.code_digest RETURNING code_digest`, o.ID, digest[:]).Scan(&stored)
	if err == nil && !hmac.Equal(stored, digest[:]) {
		return "", errors.New("pickup key does not match issued code")
	}
	return code, err
}

type RedeemInput struct {
	Point int64  `json:"pickup_point_id"`
	Event string `json:"event_id"`
	Code  string `json:"code"`
}

type RedemptionResult struct {
	Event  string    `json:"event_id"`
	At     time.Time `json:"redeemed_at"`
	Leader int64     `json:"-"`
}

func Redeem(ctx context.Context, tx *sql.Tx, o order.Order, leader int64, input RedeemInput) (RedemptionResult, bool, error) {
	var result RedemptionResult
	event, err := uuid.Parse(input.Event)
	if err != nil || event == uuid.Nil || input.Point <= 0 || len(input.Code) != 43 {
		return result, false, dbtx.ErrInvalid
	}
	input.Event = event.String()
	var digest []byte
	var arrived, redeemed sql.NullTime
	var previous string
	err = tx.QueryRowContext(ctx, `SELECT r.code_digest,b.arrived_at,COALESCE(r.event_id::text,''),r.redeemed_at
		FROM pickup_redemptions r JOIN group_batches b USING (campaign_id)
		JOIN pickup_point_leaders p ON p.pickup_point_id=r.pickup_point_id AND p.leader_id=r.leader_id
		WHERE r.order_id=$1 AND r.leader_id=$2 AND r.pickup_point_id=$3 AND p.active FOR SHARE OF p`, o.ID, leader, input.Point).Scan(&digest, &arrived, &previous, &redeemed)
	if err != nil {
		return result, false, err
	}
	if o.CanceledAt.Valid && previous == "" {
		return result, false, dbtx.ErrConflict
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return result, false, err
	}
	p := Pickup{OrderID: strconv.FormatInt(o.ID, 10), BatchID: strconv.FormatInt(o.Campaign, 10),
		PickupPointID: strconv.FormatInt(input.Point, 10), LeaderID: strconv.FormatInt(leader, 10), ArrivedAt: arrived.Time,
		CodeDigest: [sha256.Size]byte(digest), eventID: previous, redeemedAt: redeemed.Time}
	changed, err := p.Redeem(Redemption{OrderID: p.OrderID, PickupPointID: p.PickupPointID, LeaderID: p.LeaderID, Code: input.Code, EventID: input.Event}, now)
	if err != nil {
		return result, false, err
	}
	result = RedemptionResult{Event: input.Event, At: p.RedeemedAt(), Leader: leader}
	if changed {
		_, err = tx.ExecContext(ctx, "UPDATE pickup_redemptions SET event_id=$2,redeemed_at=$3 WHERE order_id=$1", o.ID, input.Event, result.At)
	}
	return result, changed, err
}

// RestoreIfUnaccepted returns merchandise only when the merchant has not started fulfillment.
func RestoreIfUnaccepted(ctx context.Context, tx *sql.Tx, o order.Order) error {
	var started bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM merchant_deliveries WHERE order_id=$1)`, o.ID).Scan(&started); err != nil || started {
		return err
	}
	items, err := order.Items(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	for _, item := range items {
		if _, err := inventory.ReleaseConsumed(ctx, tx, o.ID, item.SKU); err != nil {
			return err
		}
	}
	return nil
}

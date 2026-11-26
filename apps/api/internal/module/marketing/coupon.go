package marketing

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

func Coupon(ctx context.Context, tx *sql.Tx, store int64, code string, subtotal int64, now time.Time) (id, discount int64, err error) {
	err = tx.QueryRowContext(ctx, `
		SELECT id, LEAST(discount_cents, $3) FROM coupons
		WHERE store_id=$1 AND code=$2 AND minimum_cents <= $3 AND active AND expires_at>$4
		FOR SHARE`, store, code, subtotal, now).Scan(&id, &discount)
	return
}

func Use(ctx context.Context, tx *sql.Tx, coupon, order int64, buyer string) error {
	return dbtx.Changed(tx.ExecContext(ctx, `
		INSERT INTO coupon_uses (coupon_id,buyer_id,order_id) VALUES ($1,$2,$3)
		ON CONFLICT (coupon_id,buyer_id) DO NOTHING`, coupon, buyer, order))
}

func Release(ctx context.Context, tx *sql.Tx, order int64) error {
	_, err := tx.ExecContext(ctx, "DELETE FROM coupon_uses WHERE order_id=$1", order)
	return err
}

type Input struct {
	Code      string    `json:"code"`
	Discount  int64     `json:"discount_cents"`
	Minimum   int64     `json:"minimum_cents"`
	ExpiresAt time.Time `json:"expires_at"`
	Active    *bool     `json:"active"`
}

func Save(ctx context.Context, tx *sql.Tx, store, id int64, input Input) (int64, error) {
	code := strings.TrimSpace(input.Code)
	if code == "" || len(code) > 128 || input.Discount <= 0 || input.Minimum < 0 || input.ExpiresAt.IsZero() {
		return 0, dbtx.ErrInvalid
	}
	query := `INSERT INTO coupons (store_id,code,discount_cents,minimum_cents,expires_at,active)
		VALUES ($1,$2,$3,$4,$5,COALESCE($6,true)) RETURNING id`
	args := []any{store, code, input.Discount, input.Minimum, input.ExpiresAt, input.Active}
	if id != 0 {
		query = `UPDATE coupons SET code=$2,discount_cents=$3,minimum_cents=$4,expires_at=$5,active=COALESCE($6,active)
			WHERE id=$7 AND store_id=$1 RETURNING id`
		args = append(args, id)
	}
	err := tx.QueryRowContext(ctx, query, args...).Scan(&id)
	return id, err
}

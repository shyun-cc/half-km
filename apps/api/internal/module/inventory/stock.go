package inventory

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
)

func Set(ctx context.Context, tx *sql.Tx, store, sku, onHand int64) error {
	if onHand < 0 {
		return dbtx.ErrInvalid
	}
	return dbtx.Changed(tx.ExecContext(ctx, `
		INSERT INTO stock (sku_id, on_hand)
		SELECT id, $3 FROM skus WHERE id=$1 AND store_id=$2
		ON CONFLICT (sku_id) DO UPDATE SET on_hand=EXCLUDED.on_hand
		WHERE stock.reserved <= EXCLUDED.on_hand`, sku, store, onHand))
}

func Hold(ctx context.Context, tx *sql.Tx, sku, quantity int64) error {
	return dbtx.Changed(tx.ExecContext(ctx, `
		UPDATE stock SET reserved=reserved+$2 WHERE sku_id=$1 AND on_hand-reserved >= $2`, sku, quantity))
}

func Take(ctx context.Context, tx *sql.Tx, order, sku, quantity int64, cutoff time.Time) error {
	if err := dbtx.Changed(tx.ExecContext(ctx, `
		UPDATE stock SET on_hand=on_hand-$2 WHERE sku_id=$1 AND on_hand-reserved >= $2`, sku, quantity)); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO stock_reservations (order_id,sku_id,quantity,state,expires_at)
		VALUES ($1,$2,$3,'consumed',$4)`, order, sku, quantity, cutoff)
	return err
}

func ReleaseConsumed(ctx context.Context, tx *sql.Tx, order, sku int64) (int64, error) {
	var quantity int64
	err := tx.QueryRowContext(ctx, `UPDATE stock_reservations SET state='released'
		WHERE order_id=$1 AND sku_id=$2 AND state='consumed' RETURNING quantity`, order, sku).Scan(&quantity)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return quantity, dbtx.Changed(tx.ExecContext(ctx, "UPDATE stock SET on_hand=on_hand+$2 WHERE sku_id=$1", sku, quantity))
}

func ResolveHolds(ctx context.Context, tx *sql.Tx, orderID int64, consume bool) error {
	// Lock shared stock in the same SKU order as checkout and group payment.
	rows, err := tx.QueryContext(ctx, `
		SELECT s.sku_id FROM stock s JOIN stock_reservations r USING (sku_id)
		WHERE r.order_id=$1 AND r.state='held' ORDER BY s.sku_id FOR NO KEY UPDATE OF s`, orderID)
	if err != nil {
		return err
	}
	for rows.Next() {
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	_, err = tx.ExecContext(ctx, `
		WITH resolved AS (
			UPDATE stock_reservations SET state=CASE WHEN $2 THEN 'consumed' ELSE 'released' END
			WHERE order_id=$1 AND state='held' RETURNING sku_id, quantity
		)
		UPDATE stock s SET reserved=s.reserved-r.quantity,
			on_hand=s.on_hand-CASE WHEN $2 THEN r.quantity ELSE 0 END
		FROM resolved r WHERE s.sku_id=r.sku_id`, orderID, consume)
	return err
}

func RecordHold(ctx context.Context, tx *sql.Tx, order, sku, quantity int64, until time.Time) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO stock_reservations (order_id,sku_id,quantity,state,expires_at) VALUES ($1,$2,$3,'held',$4)`, order, sku, quantity, until)
	return err
}

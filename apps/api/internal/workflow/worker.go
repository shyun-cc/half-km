package workflow

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/groupbuy"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
	"github.com/shyun-cc/half-km/apps/api/internal/module/payment"
)

func RunWorker(ctx context.Context, db *sql.DB) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pass, cancel := context.WithTimeout(ctx, 20*time.Second)
			if err := Sweep(pass, db); err != nil && ctx.Err() == nil {
				var pg *pgconn.PgError
				if errors.As(err, &pg) {
					log.Printf("workflow sweep failed (SQLSTATE %s)", pg.Code)
				} else {
					log.Printf("workflow sweep failed (%T)", err)
				}
			}
			cancel()
		}
	}
}

// Sweep performs one bounded pass. Each campaign or expiration has its own transaction.
func Sweep(ctx context.Context, db *sql.DB) error {
	ids, err := dueIDs(ctx, db, `SELECT id FROM group_campaigns c WHERE cutoff_at<=clock_timestamp()
		AND NOT EXISTS (SELECT 1 FROM group_batches WHERE campaign_id=c.id)
		ORDER BY cutoff_at,id LIMIT 100`)
	if err != nil {
		return err
	}
	var failures []error
	for _, id := range ids {
		_, err := dbtx.Run(ctx, db, func(tx *sql.Tx) (struct{}, error) {
			_, err := groupbuy.Close(ctx, tx, id, 0)
			return struct{}{}, err
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	ids, err = dueIDs(ctx, db, `SELECT o.id FROM orders o WHERE o.hold_until<=clock_timestamp()
		AND o.canceled_at IS NULL AND NOT EXISTS (SELECT 1 FROM payments WHERE order_id=o.id)
		ORDER BY o.hold_until,o.id LIMIT 100`)
	if err != nil {
		return errors.Join(append(failures, err)...)
	}
	for _, id := range ids {
		_, err := dbtx.Run(ctx, db, func(tx *sql.Tx) (struct{}, error) {
			o, err := order.Get(ctx, tx, id, true)
			if err != nil || o.CanceledAt.Valid {
				return struct{}{}, err
			}
			_, err = payment.Get(ctx, tx, id)
			if !errors.Is(err, sql.ErrNoRows) {
				return struct{}{}, err
			}
			now, err := dbtx.Now(ctx, tx)
			if err != nil || !o.HoldUntil.Valid || now.Before(o.HoldUntil.Time) {
				return struct{}{}, err
			}
			return struct{}{}, order.Cancel(ctx, tx, o, now)
		})
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func dueIDs(ctx context.Context, db *sql.DB, query string) ([]int64, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		result = append(result, id)
	}
	return result, rows.Err()
}

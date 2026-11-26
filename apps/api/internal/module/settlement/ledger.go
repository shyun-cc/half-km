package settlement

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
)

func lockLedger(ctx context.Context, tx *sql.Tx, store, leader int64) error {
	return dbtx.Lock(ctx, tx, fmt.Sprintf("settlement/%d/%d", store, leader))
}

func Earn(ctx context.Context, tx *sql.Tx, o order.Order, leader, refunded int64, eligible time.Time) error {
	commission, err := CommissionCents(o.Payable-o.Fee-refunded, o.Rate)
	if err != nil {
		return err
	}
	if err := lockLedger(ctx, tx, o.Store, leader); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO commission_entries (order_id,campaign_id,store_id,leader_id,event_key,entry_kind,amount_cents,eligible_at)
		VALUES ($1,$2,$3,$4,'earned','earned',$5,$6)`, o.ID, o.Campaign, o.Store, leader, commission, eligible)
	return err
}

func Reverse(ctx context.Context, tx *sql.Tx, o order.Order, refundID, totalRefund int64, now time.Time) error {
	var leader, current int64
	var eligible time.Time
	err := tx.QueryRowContext(ctx, `SELECT leader_id,eligible_at,(SELECT SUM(amount_cents) FROM commission_entries WHERE order_id=$1)
		FROM commission_entries WHERE order_id=$1 AND entry_kind='earned'`, o.ID).Scan(&leader, &eligible, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	remaining, err := CommissionCents(o.Payable-o.Fee-totalRefund, o.Rate)
	if err != nil || remaining == current {
		return err
	}
	if err := lockLedger(ctx, tx, o.Store, leader); err != nil {
		return err
	}
	if now.After(eligible) {
		eligible = now
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO commission_entries (order_id,campaign_id,store_id,leader_id,event_key,entry_kind,amount_cents,eligible_at)
		VALUES ($1,$2,$3,$4,$5,'refund_adjustment',$6,$7)`, o.ID, o.Campaign, o.Store, leader, fmt.Sprintf("refund/%d", refundID), remaining-current, eligible)
	return err
}

type Period struct {
	Leader int64  `json:"leader_id"`
	Start  string `json:"period_start"`
	End    string `json:"period_end"`
}

type Statement struct {
	ID        int64      `json:"id"`
	Leader    int64      `json:"leader_id"`
	Start     string     `json:"period_start"`
	End       string     `json:"period_end"`
	Total     int64      `json:"total_cents"`
	Reference string     `json:"offline_payment_reference,omitempty"`
	PaidAt    *time.Time `json:"offline_paid_at,omitempty"`
}

const statementColumns = `id,leader_id,period_start::text,period_end::text,total_cents,
	COALESCE(offline_payment_reference,''),offline_paid_at`

func readStatement(row *sql.Row) (Statement, error) {
	var s Statement
	err := row.Scan(&s.ID, &s.Leader, &s.Start, &s.End, &s.Total, &s.Reference, &s.PaidAt)
	return s, err
}

func Generate(ctx context.Context, tx *sql.Tx, store int64, input Period) (Statement, error) {
	start, err := time.Parse(time.DateOnly, input.Start)
	if err != nil {
		return Statement{}, dbtx.ErrInvalid
	}
	end, err := time.Parse(time.DateOnly, input.End)
	if err != nil || !end.After(start) || input.Leader <= 0 {
		return Statement{}, dbtx.ErrInvalid
	}
	if err := lockLedger(ctx, tx, store, input.Leader); err != nil {
		return Statement{}, err
	}
	s, err := readStatement(tx.QueryRowContext(ctx, `SELECT `+statementColumns+` FROM commission_statements
		WHERE store_id=$1 AND leader_id=$2 AND period_start=$3::date AND period_end=$4::date`, store, input.Leader, input.Start, input.End))
	if !errors.Is(err, sql.ErrNoRows) {
		return s, err
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return Statement{}, err
	}
	if end.After(now) {
		return Statement{}, dbtx.ErrConflict
	}
	var overlap bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM commission_statements WHERE store_id=$1 AND leader_id=$2 AND period_end>$3::date)`,
		store, input.Leader, input.Start).Scan(&overlap)
	if err != nil {
		return Statement{}, err
	}
	if overlap {
		return Statement{}, dbtx.ErrConflict
	}

	// Unassigned entries before the end include earlier adjustments carried forward.
	var total int64
	err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0) FROM commission_entries
		WHERE store_id=$1 AND leader_id=$2 AND eligible_at<$3 AND statement_id IS NULL`, store, input.Leader, end).Scan(&total)
	if err != nil {
		return Statement{}, err
	}
	if total <= 0 {
		return Statement{}, dbtx.ErrConflict
	}
	s, err = readStatement(tx.QueryRowContext(ctx, `INSERT INTO commission_statements (store_id,leader_id,period_start,period_end,total_cents)
		VALUES ($1,$2,$3,$4,$5) RETURNING `+statementColumns, store, input.Leader, input.Start, input.End, total))
	if err != nil {
		return Statement{}, err
	}
	_, err = tx.ExecContext(ctx, `UPDATE commission_entries SET statement_id=$4
		WHERE store_id=$1 AND leader_id=$2 AND eligible_at<$3 AND statement_id IS NULL`, store, input.Leader, end, s.ID)
	return s, err
}

type Payout struct {
	Reference string    `json:"offline_payment_reference"`
	Amount    int64     `json:"amount_cents"`
	PaidAt    time.Time `json:"offline_paid_at"`
}

func RegisterPayout(ctx context.Context, tx *sql.Tx, store, id int64, input Payout) (Statement, error) {
	input.Reference = strings.TrimSpace(input.Reference)
	input.PaidAt = input.PaidAt.UTC().Truncate(time.Microsecond)
	if input.Reference == "" || len(input.Reference) > 128 || input.PaidAt.IsZero() {
		return Statement{}, dbtx.ErrInvalid
	}
	s, err := readStatement(tx.QueryRowContext(ctx, `SELECT `+statementColumns+` FROM commission_statements
		WHERE id=$1 AND store_id=$2 FOR NO KEY UPDATE`, id, store))
	if err != nil {
		return Statement{}, err
	}
	if s.Total != input.Amount || s.Total <= 0 {
		return Statement{}, dbtx.ErrConflict
	}
	if s.PaidAt != nil {
		if s.Reference != input.Reference || !s.PaidAt.Equal(input.PaidAt) {
			return Statement{}, dbtx.ErrConflict
		}
		return s, nil
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return Statement{}, err
	}
	if input.PaidAt.After(now) {
		return Statement{}, dbtx.ErrInvalid
	}
	return readStatement(tx.QueryRowContext(ctx, `UPDATE commission_statements SET offline_payment_reference=$2,offline_paid_at=$3
		WHERE id=$1 RETURNING `+statementColumns, id, input.Reference, input.PaidAt))
}

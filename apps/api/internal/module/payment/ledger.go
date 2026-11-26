package payment

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
)

type Input struct {
	Account     int64     `json:"receiving_account_id"`
	Transaction string    `json:"transaction_id"`
	Amount      int64     `json:"amount_cents"`
	PaidAt      time.Time `json:"paid_at"`
}

type Recorded struct {
	Transaction string    `json:"transaction_id"`
	Amount      int64     `json:"amount_cents"`
	Disposition string    `json:"disposition"`
	PaidAt      time.Time `json:"paid_at"`
	RecordedAt  time.Time `json:"recorded_at"`
}

func Get(ctx context.Context, tx *sql.Tx, id int64) (Recorded, error) {
	var result Recorded
	err := tx.QueryRowContext(ctx, `SELECT provider_transaction_id,amount_cents,disposition,paid_at,recorded_at
		FROM payments WHERE order_id=$1`, id).Scan(&result.Transaction, &result.Amount, &result.Disposition, &result.PaidAt, &result.RecordedAt)
	return result, err
}

// Apply runs with the order and its campaign already locked. Claim is called only for
// a first receipt and may reserve fulfillment resources within the same transaction.
func Apply(ctx context.Context, tx *sql.Tx, o order.Order, input Input, claim func() error) (Recorded, error) {
	if input.Account != o.Account || input.Amount != o.Payable || input.Transaction == "" ||
		strings.TrimSpace(input.Transaction) != input.Transaction || len(input.Transaction) > 128 || input.PaidAt.IsZero() {
		return Recorded{}, ErrConflict
	}
	previous, err := Get(ctx, tx, o.ID)
	if err == nil {
		if previous.Transaction != input.Transaction || previous.Amount != input.Amount {
			return Recorded{}, ErrConflict
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Recorded{}, err
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return Recorded{}, err
	}

	p := Payment{OrderID: strconv.FormatInt(o.ID, 10), AccountID: strconv.FormatInt(o.Account, 10), AmountCents: o.Payable, Cutoff: o.Cutoff, State: Pending}
	if o.CanceledAt.Valid || (o.HoldUntil.Valid && !now.Before(o.HoldUntil.Time)) {
		p.State = Canceled
	}
	accepted, err := p.Record(Receipt{OrderID: p.OrderID, AccountID: p.AccountID, TransactionID: input.Transaction, AmountCents: input.Amount, PaidAt: input.PaidAt}, now)
	if err != nil && !errors.Is(err, ErrClosed) {
		return Recorded{}, err
	}
	if accepted {
		accepted, now, err = claimBeforeExpiry(ctx, tx, o, now, claim)
		if err != nil {
			return Recorded{}, err
		}
	}

	result := Recorded{Transaction: input.Transaction, Amount: input.Amount, Disposition: "accepted", RecordedAt: now}
	if !accepted {
		result.Disposition = "refund_required"
		if err := order.Cancel(ctx, tx, o, now); err != nil {
			return Recorded{}, err
		}
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO payments (order_id,receiving_account_id,provider_transaction_id,
		amount_cents,paid_at,disposition,recorded_at) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING paid_at`,
		o.ID, o.Account, input.Transaction, input.Amount, input.PaidAt, result.Disposition, now).Scan(&result.PaidAt)
	return result, err
}

// Recheck the server time after acquiring stock. A receipt that ran out of
// time or capacity is retained while its fulfillment writes are rolled back.
func claimBeforeExpiry(ctx context.Context, tx *sql.Tx, o order.Order, initial time.Time, claim func() error) (bool, time.Time, error) {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT fulfillment_claim"); err != nil {
		return false, initial, err
	}
	claimErr := claim()
	if claimErr != nil && !errors.Is(claimErr, dbtx.ErrConflict) {
		return false, initial, claimErr
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return false, initial, err
	}
	if now.Before(initial) {
		now = initial
	}
	accepted := claimErr == nil && (o.Cutoff.IsZero() || now.Before(o.Cutoff)) &&
		(!o.HoldUntil.Valid || now.Before(o.HoldUntil.Time))
	if !accepted {
		if _, err := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT fulfillment_claim"); err != nil {
			return false, now, err
		}
	}
	_, err = tx.ExecContext(ctx, "RELEASE SAVEPOINT fulfillment_claim")
	return accepted, now, err
}

type RefundInput struct {
	Key         string `json:"request_key"`
	Reference   string `json:"provider_refund_id"`
	Transaction string `json:"transaction_id"`
	Account     int64  `json:"receiving_account_id"`
	Amount      int64  `json:"amount_cents"`
}

type Refund struct {
	ID     int64 `json:"id"`
	Total  int64 `json:"refunded_cents"`
	Amount int64 `json:"amount_cents"`
}

func Refunded(ctx context.Context, tx *sql.Tx, id int64) (int64, error) {
	var total int64
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(amount_cents),0) FROM refunds WHERE order_id=$1 AND state='succeeded'`, id).Scan(&total)
	return total, err
}

// RegisterRefund records an independently verified, completed original-route refund.
func RegisterRefund(ctx context.Context, tx *sql.Tx, o order.Order, input RefundInput) (Refund, bool, error) {
	var result Refund
	input.Key = strings.TrimSpace(input.Key)
	input.Reference = strings.TrimSpace(input.Reference)
	if input.Key == "" || input.Reference == "" || len(input.Key) > 128 || len(input.Reference) > 128 || input.Amount <= 0 || input.Account != o.Account {
		return result, false, dbtx.ErrInvalid
	}
	p, err := Get(ctx, tx, o.ID)
	if err != nil {
		return result, false, err
	}
	if p.Transaction != input.Transaction {
		return result, false, ErrConflict
	}
	result.Total, err = Refunded(ctx, tx, o.ID)
	if err != nil {
		return result, false, err
	}
	var reference, state string
	err = tx.QueryRowContext(ctx, `SELECT id,COALESCE(provider_refund_id,''),amount_cents,state FROM refunds
		WHERE order_id=$1 AND request_key=$2`, o.ID, input.Key).Scan(&result.ID, &reference, &result.Amount, &state)
	if err == nil {
		if reference != input.Reference || result.Amount != input.Amount || state != "succeeded" {
			return Refund{}, false, ErrConflict
		}
		return result, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Refund{}, false, err
	}
	if input.Amount > p.Amount-result.Total {
		return Refund{}, false, ErrConflict
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO refunds (order_id,receiving_account_id,request_key,provider_refund_id,amount_cents,state)
		VALUES ($1,$2,$3,$4,$5,'succeeded') RETURNING id`, o.ID, o.Account, input.Key, input.Reference, input.Amount).Scan(&result.ID)
	result.Total += input.Amount
	result.Amount = input.Amount
	return result, true, err
}

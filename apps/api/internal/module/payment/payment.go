// Package payment applies verified receipts to an order payment.
package payment

import (
	"errors"
	"time"
)

var (
	ErrConflict = errors.New("receipt conflicts with the order payment")
	ErrClosed   = errors.New("order cannot accept payment for fulfillment")
)

type State string

const (
	Pending  State = "pending"
	Paid     State = "paid"
	Canceled State = "canceled"
)

type Receipt struct {
	OrderID       string
	AccountID     string
	TransactionID string
	AmountCents   int64
	PaidAt        time.Time
}

type Payment struct {
	OrderID     string
	AccountID   string
	AmountCents int64
	// Cutoff is exclusive. A zero value indicates an immediate order.
	Cutoff time.Time
	State  State

	receipt    Receipt
	recordedAt time.Time
}

// Record returns true only on the first successful transition. recordedAt is
// the server time after acquiring the campaign lock for a group order.
func (p *Payment) Record(receipt Receipt, recordedAt time.Time) (bool, error) {
	if p.OrderID == "" || p.AccountID == "" || receipt.OrderID != p.OrderID ||
		receipt.AccountID != p.AccountID || receipt.AmountCents != p.AmountCents ||
		receipt.TransactionID == "" || receipt.AmountCents < 0 || receipt.PaidAt.IsZero() ||
		recordedAt.IsZero() || receipt.PaidAt.After(recordedAt) {
		return false, ErrConflict
	}
	if p.State == Paid {
		if receipt.TransactionID != p.receipt.TransactionID || !receipt.PaidAt.Equal(p.receipt.PaidAt) {
			return false, ErrConflict
		}
		return false, nil
	}
	if p.State != Pending || (!p.Cutoff.IsZero() && !recordedAt.Before(p.Cutoff)) {
		return false, ErrClosed
	}

	p.receipt = receipt
	p.recordedAt = recordedAt
	p.State = Paid
	return true, nil
}

func (p Payment) IncludedAt(cutoff time.Time) bool {
	return p.State == Paid && !p.recordedAt.IsZero() && p.recordedAt.Before(cutoff)
}

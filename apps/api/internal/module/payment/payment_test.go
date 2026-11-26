package payment

import (
	"errors"
	"testing"
	"time"
)

func TestRecordIdempotently(t *testing.T) {
	now := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	p := Payment{OrderID: "order", AccountID: "account", AmountCents: 500, State: Pending, Cutoff: now.Add(time.Minute)}
	receipt := Receipt{OrderID: "order", AccountID: "account", TransactionID: "trade", AmountCents: 500, PaidAt: now}

	for _, alter := range []func(*Receipt){
		func(r *Receipt) { r.AmountCents++ },
		func(r *Receipt) { r.AccountID = "different" },
		func(r *Receipt) { r.OrderID = "different" },
	} {
		bad := receipt
		alter(&bad)
		if changed, err := p.Record(bad, now); changed || !errors.Is(err, ErrConflict) || p.State != Pending {
			t.Fatalf("invalid receipt changed payment: %v, %v, %v", changed, err, p.State)
		}
	}
	if changed, err := p.Record(receipt, now); !changed || err != nil || p.State != Paid {
		t.Fatalf("first receipt: %v, %v, %v", changed, err, p.State)
	}
	if changed, err := p.Record(receipt, p.Cutoff); changed || err != nil {
		t.Fatalf("replayed receipt after cutoff: %v, %v", changed, err)
	}
	receipt.TransactionID = "other-trade"
	if changed, err := p.Record(receipt, now); changed || !errors.Is(err, ErrConflict) {
		t.Fatalf("different receipt: %v, %v", changed, err)
	}
}

func TestRecordRejectsCutoffAndCancellation(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC)
	receipt := Receipt{OrderID: "order", AccountID: "account", TransactionID: "trade", AmountCents: 500, PaidAt: cutoff.Add(-time.Second)}
	for _, state := range []State{Pending, Canceled} {
		p := Payment{OrderID: "order", AccountID: "account", AmountCents: 500, State: state, Cutoff: cutoff}
		recordedAt := cutoff
		if state == Canceled {
			recordedAt = receipt.PaidAt
		}
		if changed, err := p.Record(receipt, recordedAt); changed || !errors.Is(err, ErrClosed) || p.State != state {
			t.Fatalf("%s payment changed: %v, %v, %v", state, changed, err, p.State)
		}
	}
}

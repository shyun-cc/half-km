// Package workflow coordinates cross-module application transactions.
package workflow

import (
	"context"
	"database/sql"
	"time"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"

	"github.com/shyun-cc/half-km/apps/api/internal/module/delivery"
	"github.com/shyun-cc/half-km/apps/api/internal/module/groupbuy"
	"github.com/shyun-cc/half-km/apps/api/internal/module/inventory"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
	"github.com/shyun-cc/half-km/apps/api/internal/module/payment"
	"github.com/shyun-cc/half-km/apps/api/internal/module/settlement"
)

func Pay(ctx context.Context, tx *sql.Tx, o order.Order, input payment.Input) (payment.Recorded, error) {
	return payment.Apply(ctx, tx, o, input, func() error {
		if o.Campaign != 0 {
			return groupbuy.Claim(ctx, tx, o)
		}
		return inventory.ResolveHolds(ctx, tx, o.ID, true)
	})
}

func Redeem(ctx context.Context, tx *sql.Tx, o order.Order, leader int64, input delivery.RedeemInput, afterSales time.Duration) (delivery.RedemptionResult, error) {
	r, changed, err := delivery.Redeem(ctx, tx, o, leader, input)
	if err != nil || !changed {
		return r, err
	}
	total, err := payment.Refunded(ctx, tx, o.ID)
	if err != nil {
		return r, err
	}
	return r, settlement.Earn(ctx, tx, o, leader, total, r.At.Add(afterSales))
}

func Refund(ctx context.Context, tx *sql.Tx, o order.Order, input payment.RefundInput) (payment.Refund, error) {
	result, changed, err := payment.RegisterRefund(ctx, tx, o, input)
	if err != nil || !changed {
		return result, err
	}
	now, err := dbtx.Now(ctx, tx)
	if err != nil {
		return result, err
	}
	if o.Campaign != 0 {
		if !now.Before(o.Cutoff) {
			if _, err := groupbuy.Close(ctx, tx, o.Campaign, o.Store); err != nil {
				return result, err
			}
		}
		if err := settlement.Reverse(ctx, tx, o, result.ID, result.Total, now); err != nil {
			return result, err
		}
	}
	if result.Total != o.Payable || o.CanceledAt.Valid {
		return result, nil
	}
	if o.Campaign != 0 {
		if err := groupbuy.ReleaseBeforeClose(ctx, tx, o); err != nil {
			return result, err
		}
	} else if err := delivery.RestoreIfUnaccepted(ctx, tx, o); err != nil {
		return result, err
	}
	return result, order.Cancel(ctx, tx, o, now)
}

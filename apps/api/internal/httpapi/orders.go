package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/delivery"
	"github.com/shyun-cc/half-km/apps/api/internal/module/order"
	"github.com/shyun-cc/half-km/apps/api/internal/module/payment"
	"github.com/shyun-cc/half-km/apps/api/internal/workflow"
)

type deliveryInput struct {
	State string `json:"state"`
}
type pickupCode struct {
	Order int64  `json:"order_id"`
	Code  string `json:"code"`
}

func forOrder[I any, O any](api API, server *ghttp.Server, route string, roles []string, lock bool,
	action func(context.Context, *sql.Tx, request, order.Order, I) (O, error)) {
	bind(api, server, route, roles, func(ctx context.Context, tx *sql.Tx, r request, input I) (O, error) {
		var zero O
		id, err := r.id("order")
		if err != nil {
			return zero, err
		}
		// Authorize the immutable owner before taking any workflow locks.
		o, err := order.Get(ctx, tx, id, false)
		if err != nil {
			return zero, err
		}
		switch r.user.Role {
		case "merchant", "payments":
			if r.user.ID != o.Store {
				return zero, sql.ErrNoRows
			}
		case "buyer":
			if r.user.Buyer != o.Buyer {
				return zero, sql.ErrNoRows
			}
		}
		if lock {
			o, err = order.Get(ctx, tx, id, true)
			if err != nil {
				return zero, err
			}
		}
		return action(ctx, tx, r, o, input)
	})
}

func (api API) bindOrders(server *ghttp.Server) {
	bind(api, server, "POST:/v1/orders", buyerRole, func(ctx context.Context, tx *sql.Tx, r request, input order.Checkout) (json.RawMessage, error) {
		o, err := order.Place(ctx, tx, r.user.Buyer, input, api.holdDuration)
		if err != nil {
			return nil, err
		}
		return order.Details(ctx, tx, o.ID)
	})
	forOrder(api, server, "GET:/v1/orders/:order", []string{"buyer", "merchant"}, false,
		func(ctx context.Context, tx *sql.Tx, _ request, o order.Order, _ empty) (json.RawMessage, error) {
			return order.Details(ctx, tx, o.ID)
		})
	forOrder(api, server, "POST:/v1/orders/:order/payment", []string{"payments"}, true,
		func(ctx context.Context, tx *sql.Tx, _ request, o order.Order, input payment.Input) (payment.Recorded, error) {
			return workflow.Pay(ctx, tx, o, input)
		})
	forOrder(api, server, "POST:/v1/orders/:order/refunds", []string{"payments"}, true,
		func(ctx context.Context, tx *sql.Tx, _ request, o order.Order, input payment.RefundInput) (payment.Refund, error) {
			return workflow.Refund(ctx, tx, o, input)
		})
	forOrder(api, server, "POST:/v1/orders/:order/cancel", buyerRole, true,
		func(ctx context.Context, tx *sql.Tx, _ request, o order.Order, _ empty) (empty, error) {
			if o.CanceledAt.Valid {
				return empty{}, nil
			}
			_, err := payment.Get(ctx, tx, o.ID)
			if err == nil {
				return empty{}, dbtx.ErrConflict
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return empty{}, err
			}
			now, err := dbtx.Now(ctx, tx)
			if err != nil {
				return empty{}, err
			}
			return empty{}, order.Cancel(ctx, tx, o, now)
		})
	forOrder(api, server, "PUT:/v1/orders/:order/delivery", merchantRole, true,
		func(ctx context.Context, tx *sql.Tx, _ request, o order.Order, input deliveryInput) (deliveryInput, error) {
			state, err := delivery.Advance(ctx, tx, o, input.State)
			return deliveryInput{State: state}, err
		})
	forOrder(api, server, "POST:/v1/orders/:order/pickup-code", buyerRole, true,
		func(ctx context.Context, tx *sql.Tx, _ request, o order.Order, _ empty) (pickupCode, error) {
			code, err := delivery.Code(ctx, tx, o, api.auth.pickupKey)
			return pickupCode{Order: o.ID, Code: code}, err
		})
	forOrder(api, server, "POST:/v1/orders/:order/redeem", []string{"leader"}, true,
		func(ctx context.Context, tx *sql.Tx, r request, o order.Order, input delivery.RedeemInput) (delivery.RedemptionResult, error) {
			return workflow.Redeem(ctx, tx, o, r.user.ID, input, api.afterSales)
		})
}

type arrival struct {
	At time.Time `json:"arrived_at"`
}

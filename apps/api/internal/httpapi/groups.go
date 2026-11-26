package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/shyun-cc/half-km/apps/api/internal/module/delivery"
	"github.com/shyun-cc/half-km/apps/api/internal/module/groupbuy"
	"github.com/shyun-cc/half-km/apps/api/internal/module/settlement"
)

func (api API) bindGroups(server *ghttp.Server) {
	bind(api, server, "POST:/v1/stores/:store/campaigns", merchantRole,
		func(ctx context.Context, tx *sql.Tx, r request, input groupbuy.OpenInput) (identifier, error) {
			store, err := r.storeID()
			if err != nil {
				return identifier{}, err
			}
			id, err := groupbuy.Open(ctx, tx, store, input)
			return identifier{ID: id}, err
		})
	bind(api, server, "POST:/v1/stores/:store/campaigns/:campaign/close", merchantRole,
		func(ctx context.Context, tx *sql.Tx, r request, _ empty) (json.RawMessage, error) {
			store, id, err := campaignIDs(r)
			if err != nil {
				return nil, err
			}
			return groupbuy.Close(ctx, tx, id, store)
		})
	bind(api, server, "GET:/v1/stores/:store/campaigns/:campaign/batch", merchantRole,
		func(ctx context.Context, tx *sql.Tx, r request, _ empty) (json.RawMessage, error) {
			store, id, err := campaignIDs(r)
			if err != nil {
				return nil, err
			}
			var owned int64
			if err := tx.QueryRowContext(ctx, "SELECT id FROM group_campaigns WHERE id=$1 AND store_id=$2", id, store).Scan(&owned); err != nil {
				return nil, err
			}
			return groupbuy.ReadBatch(ctx, tx, owned)
		})
	bind(api, server, "POST:/v1/stores/:store/campaigns/:campaign/arrival", merchantRole,
		func(ctx context.Context, tx *sql.Tx, r request, _ empty) (arrival, error) {
			store, id, err := campaignIDs(r)
			if err != nil {
				return arrival{}, err
			}
			at, err := delivery.Arrive(ctx, tx, id, store)
			return arrival{At: at}, err
		})
	bind(api, server, "POST:/v1/stores/:store/commission-statements", merchantRole,
		func(ctx context.Context, tx *sql.Tx, r request, input settlement.Period) (settlement.Statement, error) {
			store, err := r.storeID()
			if err != nil {
				return settlement.Statement{}, err
			}
			return settlement.Generate(ctx, tx, store, input)
		})
	bind(api, server, "PUT:/v1/stores/:store/commission-statements/:statement/payout", merchantRole,
		func(ctx context.Context, tx *sql.Tx, r request, input settlement.Payout) (settlement.Statement, error) {
			store, err := r.storeID()
			if err != nil {
				return settlement.Statement{}, err
			}
			id, err := r.id("statement")
			if err != nil {
				return settlement.Statement{}, err
			}
			return settlement.RegisterPayout(ctx, tx, store, id, input)
		})
}

func campaignIDs(r request) (int64, int64, error) {
	store, err := r.storeID()
	if err != nil {
		return 0, 0, err
	}
	id, err := r.id("campaign")
	return store, id, err
}

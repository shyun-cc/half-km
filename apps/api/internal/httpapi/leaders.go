package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/shyun-cc/half-km/apps/api/internal/module/groupbuy"
)

var leaderRole = []string{"leader"}

func leaderID(r request) (any, error) { return r.user.ID, nil }

func (api API) bindLeader(server *ghttp.Server) {
	list(api, server, "leader/batches", leaderRole, leaderID, `SELECT b.campaign_id AS id,b.pickup_point_id,
		p.name AS pickup_point_name,s.name AS store_name,b.closed_at,b.arrived_at
		FROM group_batches b JOIN pickup_points p ON p.id=b.pickup_point_id
		JOIN group_campaigns c ON c.id=b.campaign_id JOIN stores s ON s.id=c.store_id
		JOIN pickup_point_leaders l ON l.pickup_point_id=b.pickup_point_id AND l.leader_id=b.leader_id
		WHERE b.leader_id=$1 AND l.active`)
	bind(api, server, "GET:/v1/leader/batches/:campaign", leaderRole,
		func(ctx context.Context, tx *sql.Tx, r request, _ empty) (json.RawMessage, error) {
			id, err := r.id("campaign")
			if err != nil {
				return nil, err
			}
			var batch int64
			err = tx.QueryRowContext(ctx, `SELECT b.campaign_id FROM group_batches b
				JOIN pickup_point_leaders l ON l.pickup_point_id=b.pickup_point_id AND l.leader_id=b.leader_id
				WHERE b.campaign_id=$1 AND b.leader_id=$2 AND l.active FOR SHARE OF l`, id, r.user.ID).Scan(&batch)
			if err != nil {
				return nil, err
			}
			return groupbuy.ReadBatch(ctx, tx, batch)
		})
	list(api, server, "leader/commission-entries", leaderRole, leaderID, `SELECT e.id,e.order_id,e.statement_id,e.entry_kind,
		e.amount_cents,e.eligible_at,s.name AS store_name FROM commission_entries e JOIN stores s ON s.id=e.store_id WHERE e.leader_id=$1`)
	list(api, server, "leader/commission-statements", leaderRole, leaderID, `SELECT c.id,c.period_start,c.period_end,c.total_cents,
		c.offline_paid_at,s.name AS store_name FROM commission_statements c JOIN stores s ON s.id=c.store_id WHERE c.leader_id=$1`)
}

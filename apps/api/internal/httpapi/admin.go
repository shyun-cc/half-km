package httpapi

import (
	"context"
	"database/sql"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/catalog"
	"github.com/shyun-cc/half-km/apps/api/internal/module/community"
	"github.com/shyun-cc/half-km/apps/api/internal/module/inventory"
	"github.com/shyun-cc/half-km/apps/api/internal/module/marketing"
	"github.com/shyun-cc/half-km/apps/api/internal/module/merchant"
)

var operator = []string{"operator"}
var storeAdmin = []string{"operator", "merchant"}
var merchantRole = []string{"merchant"}
var buyerRole = []string{"buyer"}
var users = []string{"operator", "merchant", "buyer"}

type identifier struct {
	ID int64 `json:"id"`
}
type flag struct {
	Active *bool `json:"active"`
}
type stockInput struct {
	OnHand *int64 `json:"on_hand"`
}

func save[I any](api API, server *ghttp.Server, route string, roles []string, write func(context.Context, *sql.Tx, request, int64, I) (int64, error)) {
	for _, method := range []string{"POST", "PUT"} {
		path := route
		if method == "PUT" {
			path += "/:id"
		}
		bind(api, server, method+":"+path, roles, func(ctx context.Context, tx *sql.Tx, r request, input I) (identifier, error) {
			var id int64
			if method == "PUT" {
				var err error
				id, err = r.id("id")
				if err != nil {
					return identifier{}, err
				}
			}
			id, err := write(ctx, tx, r, id, input)
			return identifier{ID: id}, err
		})
	}
}

func saveForStore[I any](api API, server *ghttp.Server, resource string, write func(context.Context, *sql.Tx, int64, int64, I) (int64, error)) {
	save(api, server, "/v1/stores/:store/"+resource, storeAdmin, func(ctx context.Context, tx *sql.Tx, r request, id int64, input I) (int64, error) {
		store, err := r.storeID()
		if err != nil {
			return 0, err
		}
		return write(ctx, tx, store, id, input)
	})
}

func (api API) bindAdmin(server *ghttp.Server) {
	for _, resource := range []string{"communities", "circles", "buildings"} {
		save(api, server, "/v1/"+resource, operator, func(ctx context.Context, tx *sql.Tx, _ request, id int64, input community.Named) (int64, error) {
			return community.Save(ctx, tx, resource, id, input)
		})
	}
	save(api, server, "/v1/stores", operator, func(ctx context.Context, tx *sql.Tx, _ request, id int64, input merchant.Store) (int64, error) {
		return merchant.SaveStore(ctx, tx, id, input)
	})
	save(api, server, "/v1/pickup-points", operator, func(ctx context.Context, tx *sql.Tx, _ request, id int64, input merchant.Point) (int64, error) {
		return merchant.SavePoint(ctx, tx, id, input)
	})
	save(api, server, "/v1/leaders", operator, func(ctx context.Context, tx *sql.Tx, _ request, id int64, input merchant.Leader) (int64, error) {
		return merchant.SaveLeader(ctx, tx, id, input)
	})
	bind(api, server, "PUT:/v1/circles/:circle/buildings/:building", operator, func(ctx context.Context, tx *sql.Tx, r request, input flag) (empty, error) {
		circle, err := r.id("circle")
		if err != nil {
			return empty{}, err
		}
		building, err := r.id("building")
		if err != nil || input.Active == nil {
			return empty{}, dbtx.ErrInvalid
		}
		return empty{}, community.AllowBuilding(ctx, tx, circle, building, *input.Active)
	})
	bind(api, server, "PUT:/v1/stores/:store/circles/:circle", storeAdmin, func(ctx context.Context, tx *sql.Tx, r request, input flag) (empty, error) {
		store, err := r.storeID()
		if err != nil {
			return empty{}, err
		}
		circle, err := r.id("circle")
		if err != nil || input.Active == nil {
			return empty{}, dbtx.ErrInvalid
		}
		return empty{}, merchant.SetCircle(ctx, tx, store, circle, *input.Active)
	})
	bind(api, server, "PUT:/v1/pickup-points/:point/leaders/:leader", operator, func(ctx context.Context, tx *sql.Tx, r request, input flag) (empty, error) {
		point, err := r.id("point")
		if err != nil {
			return empty{}, err
		}
		leader, err := r.id("leader")
		if err != nil || input.Active == nil {
			return empty{}, dbtx.ErrInvalid
		}
		return empty{}, merchant.AssignLeader(ctx, tx, point, leader, *input.Active)
	})
	saveForStore(api, server, "receiving-accounts", merchant.SaveAccount)
	saveForStore(api, server, "products", catalog.SaveProduct)
	saveForStore(api, server, "skus", catalog.SaveSKU)
	saveForStore(api, server, "coupons", marketing.Save)
	bind(api, server, "PUT:/v1/stores/:store/skus/:sku/stock", storeAdmin, func(ctx context.Context, tx *sql.Tx, r request, input stockInput) (empty, error) {
		store, err := r.storeID()
		if err != nil {
			return empty{}, err
		}
		sku, err := r.id("sku")
		if err != nil || input.OnHand == nil {
			return empty{}, dbtx.ErrInvalid
		}
		return empty{}, inventory.Set(ctx, tx, store, sku, *input.OnHand)
	})
}

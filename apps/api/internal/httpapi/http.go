// Package httpapi exposes authenticated application operations over HTTP.
package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shyun-cc/shw-gf-lib/response"

	"github.com/shyun-cc/half-km/apps/api/internal/dbtx"
	"github.com/shyun-cc/half-km/apps/api/internal/module/delivery"
	"github.com/shyun-cc/half-km/apps/api/internal/module/payment"
)

type API struct {
	db           *sql.DB
	auth         Auth
	holdDuration time.Duration
	afterSales   time.Duration
}

func Bind(server *ghttp.Server, db *sql.DB, auth Auth) {
	api := API{db: db, auth: auth, holdDuration: 15 * time.Minute, afterSales: 7 * 24 * time.Hour}
	api.bindAdmin(server)
	api.bindOrders(server)
	api.bindGroups(server)
	api.bindLists(server)
}

type empty struct{}

type request struct {
	user principal
	raw  *ghttp.Request
}

func (r request) id(name string) (int64, error) {
	id, err := strconv.ParseInt(r.raw.GetRouter(name).String(), 10, 64)
	if err != nil || id <= 0 {
		return 0, dbtx.ErrInvalid
	}
	return id, nil
}

func (r request) storeID() (int64, error) {
	id, err := r.id("store")
	if err != nil {
		return 0, err
	}
	return id, r.user.store(id)
}

func bind[I any, O any](api API, server *ghttp.Server, route string, roles []string,
	action func(context.Context, *sql.Tx, request, I) (O, error)) {
	server.BindHandler(route, func(r *ghttp.Request) {
		r.Response.Header().Set("Cache-Control", "no-store")
		p, ok := api.auth.authenticate(r.Header.Get("Authorization"))
		if !ok {
			r.Response.Header().Set("WWW-Authenticate", "Bearer")
			fail(r, http.StatusUnauthorized, "authentication required")
			return
		}
		if !slices.Contains(roles, p.Role) {
			replyError(r, dbtx.ErrDenied)
			return
		}
		var input I
		if _, noBody := any(input).(empty); !noBody {
			kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil || kind != "application/json" {
				fail(r, http.StatusUnsupportedMediaType, "expected application/json")
				return
			}
			r.Body = http.MaxBytesReader(r.Response.RawWriter(), r.Body, 64<<10)
			decoder := json.NewDecoder(r.Body)
			decoder.DisallowUnknownFields()
			if err := decodeObject(decoder, &input); err != nil {
				var size *http.MaxBytesError
				if errors.As(err, &size) {
					fail(r, http.StatusRequestEntityTooLarge, "request body too large")
					return
				}
				replyError(r, dbtx.ErrInvalid)
				return
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		result, err := dbtx.Run(ctx, api.db, func(tx *sql.Tx) (O, error) {
			return action(ctx, tx, request{user: p, raw: r}, input)
		})
		if err != nil {
			replyError(r, err)
			return
		}
		response.Success(r, result)
	})
}

func decodeObject[T any](d *json.Decoder, result *T) error {
	var input *T
	if err := d.Decode(&input); err != nil {
		return err
	}
	if input == nil {
		return dbtx.ErrInvalid
	}
	var trailing json.RawMessage
	if err := d.Decode(&trailing); err != io.EOF {
		if err != nil {
			return err
		}
		return dbtx.ErrInvalid
	}
	*result = *input
	return nil
}

func fail(r *ghttp.Request, status int, message string) {
	r.Response.WriteHeader(status)
	response.Error(r, message, status)
}

func replyError(r *ghttp.Request, err error) {
	var pg *pgconn.PgError
	switch {
	case errors.Is(err, dbtx.ErrDenied), errors.Is(err, delivery.ErrPickupDenied):
		fail(r, http.StatusForbidden, "operation not permitted")
	case errors.Is(err, sql.ErrNoRows):
		fail(r, http.StatusNotFound, "resource unavailable")
	case errors.Is(err, dbtx.ErrInvalid):
		fail(r, http.StatusBadRequest, "invalid request")
	case errors.Is(err, dbtx.ErrConflict), errors.Is(err, payment.ErrConflict), errors.Is(err, delivery.ErrAlreadyRedeemed):
		fail(r, http.StatusConflict, "operation conflicts with current state")
	case errors.As(err, &pg) && (strings.HasPrefix(pg.Code, "22") || pg.Code == "23514" || pg.Code == "23502"):
		fail(r, http.StatusBadRequest, "invalid request")
	case errors.As(err, &pg) && (strings.HasPrefix(pg.Code, "23") || pg.Code == "40001" || pg.Code == "40P01"):
		fail(r, http.StatusConflict, "operation conflicts with current state")
	case errors.Is(err, context.DeadlineExceeded):
		fail(r, http.StatusServiceUnavailable, "operation timed out")
	default:
		if errors.As(err, &pg) {
			log.Printf("business database operation failed (SQLSTATE %s)", pg.Code)
		} else {
			log.Printf("business operation failed (%T)", err)
		}
		fail(r, http.StatusInternalServerError, "operation failed")
	}
}

package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gogf/gf/v2/net/ghttp"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/shyun-cc/half-km/apps/api/internal/module/settlement"
	"github.com/shyun-cc/half-km/apps/api/internal/workflow"
)

type fixture struct {
	client                                               *http.Client
	t                                                    *testing.T
	db                                                   *sql.DB
	server                                               *ghttp.Server
	tokens                                               map[string]string
	buyer                                                string
	store, circle, building, account, point, leader, sku int64
}

type reply struct {
	status int
	body   []byte
}

func (f *fixture) request(method, path, role string, input any) reply {
	var body []byte
	if input != nil {
		var err error
		body, err = json.Marshal(input)
		if err != nil {
			panic(err)
		}
	}
	req, err := http.NewRequest(method, "http://"+f.server.GetListenedAddress()+"/v1"+path, bytes.NewReader(body))
	if err != nil {
		panic(err)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if role != "" {
		req.Header.Set("Authorization", "Bearer "+f.tokens[role])
	}
	out, err := f.client.Do(req)
	if err != nil {
		return reply{body: []byte(err.Error())}
	}
	defer out.Body.Close()
	body, err = io.ReadAll(out.Body)
	if err != nil {
		return reply{body: []byte(err.Error())}
	}
	return reply{status: out.StatusCode, body: body}
}

func read[T any](t *testing.T, r reply, status int) T {
	t.Helper()
	if r.status != status {
		t.Fatalf("HTTP %d, expected %d: %s", r.status, status, r.body)
	}
	var envelope struct {
		Code int `json:"code"`
		Data T   `json:"data"`
	}
	if err := json.Unmarshal(r.body, &envelope); err != nil {
		t.Fatalf("decode %q: %v", r.body, err)
	}
	wantCode := status
	if status == http.StatusOK {
		wantCode = 0
	}
	if envelope.Code != wantCode {
		t.Fatalf("bad app code: %s", r.body)
	}
	return envelope.Data
}

func (f *fixture) write(method, path, role string, input any) int64 {
	f.t.Helper()
	return read[identifier](f.t, f.request(method, path, role, input), 200).ID
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.ExecContext(f.t.Context(), query, args...); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) equal(want int64, query string, args ...any) {
	f.t.Helper()
	var actual int64
	if err := f.db.QueryRowContext(f.t.Context(), query, args...).Scan(&actual); err != nil || actual != want {
		f.t.Fatalf("%s: got %d, expected %d, error %v", query, actual, want, err)
	}
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("HALFKM_TEST_DATABASE_URL")
	if dsn == "" {
		if os.Getenv("CI") == "true" {
			t.Fatal("HALFKM_TEST_DATABASE_URL is required in CI")
		}
		t.Skip("set HALFKM_TEST_DATABASE_URL to a disposable PostgreSQL 18 database")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	base := stdlib.OpenDB(*config)
	t.Cleanup(func() { base.Close() })
	var version int
	if err := base.QueryRowContext(t.Context(), "SHOW server_version_num").Scan(&version); err != nil || version/10000 != 18 {
		t.Fatalf("expected PostgreSQL 18, got %d: %v", version, err)
	}
	schema := pgx.Identifier{"test_" + hex.EncodeToString(randBytes(t, 8))}.Sanitize()
	if _, err := base.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := base.ExecContext(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(12)
	t.Cleanup(func() { db.Close() })

	migrations, err := filepath.Glob("../../../../migrations/*.sql")
	if err != nil || len(migrations) < 2 {
		t.Fatalf("migration files: %v, %v", migrations, err)
	}
	for _, path := range migrations {
		sqlText, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), string(sqlText)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
	f := &fixture{t: t, db: db, server: ghttp.GetServer(schema), tokens: make(map[string]string), buyer: uuid.NewString()}
	auth := Auth{tokens: make(map[[sha256.Size]byte]principal), pickupKey: randBytes(t, 32)}
	addToken := func(label string, p principal) {
		token := hex.EncodeToString(randBytes(t, 32))
		f.tokens[label] = token
		auth.tokens[sha256.Sum256([]byte(token))] = p
	}
	addToken("operator", principal{Role: "operator"})
	addToken("buyer", principal{Role: "buyer", Buyer: f.buyer})
	addToken("other-buyer", principal{Role: "buyer", Buyer: uuid.NewString()})
	Bind(f.server, db, auth)
	f.server.SetAddr("127.0.0.1:0")
	f.server.SetDumpRouterMap(false)
	if err := f.server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.server.Shutdown() })
	f.client = &http.Client{Timeout: 12 * time.Second}
	t.Cleanup(f.client.CloseIdleConnections)

	community := f.write("POST", "/communities", "operator", map[string]any{"name": "Demo"})
	f.circle = f.write("POST", "/circles", "operator", map[string]any{"name": "North", "community_id": community})
	f.building = f.write("POST", "/buildings", "operator", map[string]any{"name": "Building A", "community_id": community})
	f.store = f.write("POST", "/stores", "operator", map[string]any{"name": "Store", "delivery_fee_cents": 100})
	f.point = f.write("POST", "/pickup-points", "operator", map[string]any{"name": "Pickup", "address": "Demo address", "circle_id": f.circle})
	f.leader = f.write("POST", "/leaders", "operator", map[string]any{"display_name": "Leader"})
	addToken("merchant", principal{Role: "merchant", ID: f.store})
	addToken("other-merchant", principal{Role: "merchant", ID: f.store + 1})
	addToken("payments", principal{Role: "payments", ID: f.store})
	addToken("leader", principal{Role: "leader", ID: f.leader})
	addToken("other-leader", principal{Role: "leader", ID: f.leader + 1})
	f.write("PUT", fmt.Sprintf("/circles/%d/buildings/%d", f.circle, f.building), "operator", map[string]bool{"active": true})
	f.write("PUT", fmt.Sprintf("/stores/%d/circles/%d", f.store, f.circle), "merchant", map[string]bool{"active": true})
	f.write("PUT", fmt.Sprintf("/pickup-points/%d/leaders/%d", f.point, f.leader), "operator", map[string]bool{"active": true})
	f.account = f.write("POST", f.shop("/receiving-accounts"), "merchant", map[string]any{"provider": "test", "merchant_number": "fixture"})
	product := f.write("POST", f.shop("/products"), "merchant", map[string]any{"name": "Produce"})
	f.sku = f.write("POST", f.shop("/skus"), "merchant", map[string]any{"name": "Apples", "product_id": product, "unit_price_cents": 1000})
	f.write("PUT", f.shop(fmt.Sprintf("/skus/%d/stock", f.sku)), "merchant", map[string]int{"on_hand": 20})
	return f
}

func randBytes(t *testing.T, n int) []byte {
	t.Helper()
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return buf
}

func (f *fixture) shop(suffix string) string { return fmt.Sprintf("/stores/%d%s", f.store, suffix) }

func orderPath(id int64, suffix string) string { return fmt.Sprintf("/orders/%d%s", id, suffix) }

func (f *fixture) checkout(key string, campaign, units int64) map[string]any {
	body := map[string]any{"checkout_key": key, "store_id": f.store, "circle_id": f.circle,
		"receiving_account_id": f.account, "items": []map[string]int64{{"sku_id": f.sku, "quantity": units}}}
	if campaign != 0 {
		body["campaign_id"] = campaign
	} else {
		body["building_id"] = f.building
		body["address"] = "Example unit"
	}
	return body
}

func (f *fixture) receipt(transaction string, amount int64) map[string]any {
	return map[string]any{"receiving_account_id": f.account, "transaction_id": transaction,
		"amount_cents": amount, "paid_at": time.Now().UTC().Add(-time.Minute)}
}

func (f *fixture) refund(order int64, transaction, reference string, amount int64) reply {
	return f.request("POST", orderPath(order, "/refunds"), "payments", map[string]any{
		"request_key": reference, "provider_refund_id": reference, "transaction_id": transaction,
		"receiving_account_id": f.account, "amount_cents": amount})
}

func concurrent(count int, run func(int) reply) []reply {
	start := make(chan struct{})
	out := make(chan reply, count)
	for index := range count {
		go func() { <-start; out <- run(index) }()
	}
	close(start)
	results := make([]reply, 0, count)
	for range count {
		results = append(results, <-out)
	}
	return results
}

func TestPostgresImmediateHTTP(t *testing.T) {
	f := newFixture(t)
	if got := read[session](t, f.request("GET", "/session", "merchant", nil), 200); got.Role != "merchant" || got.ID != f.store {
		t.Fatalf("merchant session: %+v", got)
	}
	read[any](t, f.request("GET", "/session", "payments", nil), 403)
	read[any](t, f.request("GET", "/session", "", nil), 401)
	circle := fmt.Sprintf("/circles/%d", f.circle)
	link := f.shop(circle)
	f.write("PUT", link, "operator", map[string]bool{"active": false})
	if got := read[[]identifier](t, f.request("GET", circle+"/stores", "buyer", nil), 200); len(got) != 0 {
		t.Fatalf("disabled store still listed: %+v", got)
	}
	f.write("PUT", link, "merchant", map[string]bool{"active": true})
	if got := read[[]identifier](t, f.request("GET", circle+"/stores", "buyer", nil), 200); len(got) != 1 || got[0].ID != f.store {
		t.Fatalf("enabled store missing: %+v", got)
	}
	leaders := fmt.Sprintf("/pickup-points/%d/leaders", f.point)
	if got := read[[]identifier](t, f.request("GET", leaders, "merchant", nil), 200); len(got) != 1 || got[0].ID != f.leader {
		t.Fatalf("authorized leader missing: %+v", got)
	}
	if got := read[[]identifier](t, f.request("GET", leaders, "other-merchant", nil), 200); len(got) != 0 {
		t.Fatalf("another store's leader was exposed: %+v", got)
	}

	read[any](t, f.request("GET", "/communities", "", nil), 401)
	read[any](t, f.request("POST", f.shop("/products"), "other-merchant", map[string]string{"name": "Wrong store"}), 403)
	read[any](t, f.request("GET", f.shop("/skus"), "buyer", nil), 200)
	f.write("POST", f.shop("/coupons"), "merchant", map[string]any{
		"code": "DEMO", "discount_cents": 150, "minimum_cents": 1000, "expires_at": time.Now().Add(time.Hour)})
	checkout := f.checkout("delivery-1", 0, 2)
	checkout["coupon_code"] = "DEMO"
	read[any](t, f.request("POST", "/orders", "buyer", nil), 415)
	init := f.write("POST", "/orders", "buyer", checkout)
	if again := f.write("POST", "/orders", "buyer", checkout); again != init {
		t.Fatal("checkout replay created another order")
	}
	f.equal(1950, "SELECT payable_cents FROM orders WHERE id=$1", init)
	f.equal(2, "SELECT reserved FROM stock WHERE sku_id=$1", f.sku)
	read[any](t, f.request("GET", orderPath(init, ""), "other-buyer", nil), 404)
	read[any](t, f.request("GET", orderPath(init, ""), "other-merchant", nil), 404)
	read[any](t, f.request("PUT", orderPath(init, "/delivery"), "merchant", map[string]string{"state": "accepted"}), 409)
	receipt := f.receipt("delivery-receipt", 1950)
	for _, r := range concurrent(8, func(_ int) reply { return f.request("POST", orderPath(init, "/payment"), "payments", receipt) }) {
		p := read[struct{ Disposition string }](t, r, 200)
		if p.Disposition != "accepted" {
			t.Fatalf("payment: %+v", p)
		}
	}
	read[any](t, f.request("POST", orderPath(init, "/payment"), "payments", f.receipt("wrong", 1950)), 409)
	read[any](t, f.request("POST", orderPath(init, "/payment"), "payments", f.receipt("delivery-receipt", 1951)), 409)
	f.equal(1, "SELECT count(*) FROM payments WHERE order_id=$1", init)
	f.equal(18, "SELECT on_hand FROM stock WHERE sku_id=$1", f.sku)
	f.equal(0, "SELECT reserved FROM stock WHERE sku_id=$1", f.sku)

	f.write("PUT", fmt.Sprintf("/circles/%d/buildings/%d", f.circle, f.building), "operator", map[string]bool{"active": false})
	read[any](t, f.request("POST", "/orders", "buyer", f.checkout("outside", 0, 1)), 404)
	for _, state := range []string{"accepted", "out_for_delivery", "delivered", "delivered"} {
		f.write("PUT", orderPath(init, "/delivery"), "merchant", map[string]string{"state": state})
	}
	detail := read[map[string]any](t, f.request("GET", orderPath(init, ""), "buyer", nil), 200)
	if detail["delivery_state"] != "delivered" || detail["address_snapshot"] != "Example unit" {
		t.Fatalf("order detail: %v", detail)
	}
	f.write("PUT", fmt.Sprintf("/circles/%d/buildings/%d", f.circle, f.building), "operator", map[string]bool{"active": true})
	expiring := f.write("POST", "/orders", "buyer", f.checkout("expire", 0, 1))
	f.exec("UPDATE orders SET hold_until=clock_timestamp() WHERE id=$1", expiring)
	if err := workflow.Sweep(t.Context(), f.db); err != nil {
		t.Fatal(err)
	}
	f.equal(0, "SELECT reserved FROM stock WHERE sku_id=$1", f.sku)
	late := read[struct{ Disposition string }](t, f.request("POST", orderPath(expiring, "/payment"), "payments", f.receipt("expired-receipt", 1100)), 200)
	if late.Disposition != "refund_required" {
		t.Fatalf("late payment: %+v", late)
	}
	read[any](t, f.refund(expiring, "expired-receipt", "refund-expired", 1100), 200)
	f.equal(18, "SELECT on_hand FROM stock WHERE sku_id=$1", f.sku)
}

func (f *fixture) open(quota int) int64 {
	return f.write("POST", f.shop("/campaigns"), "merchant", map[string]any{
		"circle_id": f.circle, "pickup_point_id": f.point, "leader_id": f.leader, "commission_rate_bps": 1000,
		"cutoff_at": time.Now().Add(time.Hour),
		"offers":    []map[string]any{{"sku_id": f.sku, "quota": quota}}})
}

func (f *fixture) campaign(id int64, suffix string) string {
	return f.shop(fmt.Sprintf("/campaigns/%d%s", id, suffix))
}

func (f *fixture) cutoff(id int64) {
	f.t.Helper()
	f.exec("UPDATE group_campaigns SET cutoff_at=clock_timestamp() WHERE id=$1", id)
}

type batch struct {
	Orders   []int64                    `json:"orders"`
	Lines    []struct{ Quantity int64 } `json:"sorting_lines"`
	ClosedAt time.Time                  `json:"closed_at"`
}

func TestPostgresGroupHTTP(t *testing.T) {
	f := newFixture(t)
	c := f.open(4)
	a := f.write("POST", "/orders", "buyer", f.checkout("a", c, 2))
	b := f.write("POST", "/orders", "buyer", f.checkout("b", c, 2))
	excess := f.write("POST", "/orders", "buyer", f.checkout("excess", c, 1))
	late := f.write("POST", "/orders", "buyer", f.checkout("late", c, 1))
	read[any](t, f.request("POST", f.campaign(c, "/close"), "merchant", nil), 409)
	for _, id := range []int64{a, b} {
		r := read[struct{ Disposition string }](t, f.request("POST", orderPath(id, "/payment"), "payments", f.receipt("group-"+strconv.FormatInt(id, 10), 2000)), 200)
		if r.Disposition != "accepted" {
			t.Fatalf("payment: %+v", r)
		}
	}
	r := read[struct{ Disposition string }](t, f.request("POST", orderPath(excess, "/payment"), "payments", f.receipt("excess", 1000)), 200)
	if r.Disposition != "refund_required" {
		t.Fatalf("excess payment: %+v", r)
	}
	f.equal(4, "SELECT paid_quantity FROM group_campaign_skus WHERE campaign_id=$1", c)
	f.equal(16, "SELECT on_hand FROM stock WHERE sku_id=$1", f.sku)

	// Hold the campaign row while both HTTP transactions attempt to acquire it,
	// then cross the cutoff before allowing either to proceed.
	tx, err := f.db.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(t.Context(), "SELECT 1 FROM group_campaigns WHERE id=$1 FOR NO KEY UPDATE", c); err != nil {
		t.Fatal(err)
	}
	out := make(chan reply, 2)
	go func() { out <- f.request("POST", orderPath(late, "/payment"), "payments", f.receipt("late", 1000)) }()
	go func() { out <- f.request("POST", f.campaign(c, "/close"), "merchant", nil) }()
	deadline := time.Now().Add(3 * time.Second)
	for {
		var blocked int
		err = f.db.QueryRowContext(t.Context(), `SELECT count(*) FROM pg_stat_activity
			WHERE pid<>pg_backend_pid() AND datname=current_database() AND wait_event_type='Lock'
			AND (query LIKE 'SELECT cutoff_at FROM group_campaigns%' OR query LIKE 'SELECT cutoff_at,pickup_point_id,leader_id FROM group_campaigns%')`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked >= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("payment and closing did not block on the campaign lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := tx.ExecContext(t.Context(), "UPDATE group_campaigns SET cutoff_at=clock_timestamp() WHERE id=$1", c); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		read[any](t, <-out, 200)
	}
	r = read[struct{ Disposition string }](t, f.request("POST", orderPath(late, "/payment"), "payments", f.receipt("late", 1000)), 200)
	if r.Disposition != "refund_required" {
		t.Fatalf("late payment: %+v", r)
	}
	frozen := read[batch](t, f.request("GET", f.campaign(c, "/batch"), "merchant", nil), 200)
	if len(frozen.Orders) != 2 || frozen.Orders[0] != a || frozen.Orders[1] != b || len(frozen.Lines) != 1 || frozen.Lines[0].Quantity != 4 {
		t.Fatalf("frozen batch: %+v", frozen)
	}
	for _, r := range concurrent(6, func(_ int) reply { return f.request("POST", f.campaign(c, "/close"), "merchant", nil) }) {
		if got := read[batch](t, r, 200); !got.ClosedAt.Equal(frozen.ClosedAt) {
			t.Fatalf("reclosed batch: %+v", got)
		}
	}
	code := read[pickupCode](t, f.request("POST", orderPath(a, "/pickup-code"), "buyer", nil), 200)
	input := map[string]any{"pickup_point_id": f.point, "code": code.Code, "event_id": uuid.NewString()}
	read[any](t, f.request("POST", orderPath(a, "/redeem"), "leader", input), 403)
	read[any](t, f.request("POST", orderPath(a, "/pickup-code"), "other-buyer", nil), 404)
	f.write("POST", f.campaign(c, "/arrival"), "merchant", nil)
	read[any](t, f.request("POST", orderPath(a, "/redeem"), "other-leader", input), 404)
	leaderBatch := fmt.Sprintf("/leader/batches/%d", c)
	leaderAuth := fmt.Sprintf("/pickup-points/%d/leaders/%d", f.point, f.leader)
	if got := read[[]identifier](t, f.request("GET", "/leader/batches", "leader", nil), 200); len(got) != 1 || got[0].ID != c {
		t.Fatalf("leader batches: %+v", got)
	}
	if got := read[[]identifier](t, f.request("GET", "/leader/batches", "other-leader", nil), 200); len(got) != 0 {
		t.Fatalf("another leader's batches were exposed: %+v", got)
	}
	read[any](t, f.request("GET", leaderBatch, "other-leader", nil), 404)
	read[any](t, f.request("GET", leaderBatch, "buyer", nil), 403)
	f.write("PUT", leaderAuth, "operator", map[string]bool{"active": false})
	read[any](t, f.request("GET", leaderBatch, "leader", nil), 404)
	f.write("PUT", leaderAuth, "operator", map[string]bool{"active": true})
	available := read[struct {
		Point int64      `json:"pickup_point_id"`
		At    *time.Time `json:"arrived_at"`
	}](t, f.request("GET", orderPath(a, ""), "buyer", nil), 200)
	if available.Point != f.point || available.At == nil {
		t.Fatalf("buyer pickup status: %+v", available)
	}

	for _, r := range concurrent(8, func(_ int) reply { return f.request("POST", orderPath(a, "/redeem"), "leader", input) }) {
		read[any](t, r, 200)
	}
	input["event_id"] = uuid.NewString()
	read[any](t, f.request("POST", orderPath(a, "/redeem"), "leader", input), 409)
	f.equal(1, "SELECT count(*) FROM commission_entries WHERE order_id=$1", a)
	f.equal(200, "SELECT amount_cents FROM commission_entries WHERE order_id=$1", a)
	gotBatch := read[struct {
		Orders []struct {
			At *time.Time `json:"redeemed_at"`
		} `json:"order_statuses"`
	}](t, f.request("GET", leaderBatch, "leader", nil), 200)
	if len(gotBatch.Orders) != 2 || gotBatch.Orders[0].At == nil || gotBatch.Orders[1].At != nil {
		t.Fatalf("leader batch order statuses: %+v", gotBatch)
	}
	if got := read[[]identifier](t, f.request("GET", "/leader/commission-entries", "leader", nil), 200); len(got) != 1 {
		t.Fatalf("leader commission entries: %+v", got)
	}
	if got := read[[]identifier](t, f.request("GET", "/leader/commission-entries", "other-leader", nil), 200); len(got) != 0 {
		t.Fatalf("another leader's commission was exposed: %+v", got)
	}

	f.equal(1, "SELECT count(*) FROM commission_entries e JOIN pickup_redemptions r USING (order_id) WHERE e.order_id=$1 AND e.eligible_at=r.redeemed_at+interval '168 hours'", a)

	if err := workflow.Sweep(t.Context(), f.db); err != nil {
		t.Fatal(err)
	}
	empty := f.open(1)
	f.cutoff(empty)
	if err := workflow.Sweep(t.Context(), f.db); err != nil {
		t.Fatal(err)
	}
	if got := read[batch](t, f.request("GET", f.campaign(empty, "/batch"), "merchant", nil), 200); len(got.Orders) != 0 || len(got.Lines) != 0 {
		t.Fatalf("empty batch: %+v", got)
	}
	f.testSettlement(a, b)
}

func (f *fixture) testSettlement(a, b int64) {
	t := f.t
	t.Helper()
	now := time.Now().UTC()
	end := now.Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -1)
	early := start.AddDate(0, 0, -1)
	statements := f.shop("/commission-statements")
	period := func(start, end time.Time) map[string]any {
		return map[string]any{"leader_id": f.leader, "period_start": start.Format(time.DateOnly), "period_end": end.Format(time.DateOnly)}
	}
	read[any](t, f.request("POST", statements, "merchant", period(early, start)), 409)
	// Move the existing earned entry into a completed period. Eligibility delay was
	// already verified against the actual redemption time.
	f.exec("UPDATE commission_entries SET eligible_at=$2 WHERE order_id=$1", a, early.Add(time.Hour))
	var statement int64
	for _, r := range concurrent(4, func(_ int) reply { return f.request("POST", statements, "merchant", period(early, start)) }) {
		got := read[struct {
			ID    int64
			Total int64 `json:"total_cents"`
		}](t, r, 200)
		if (statement != 0 && statement != got.ID) || got.Total != 200 {
			t.Fatalf("statement: %+v", got)
		}
		statement = got.ID
	}
	payout := fmt.Sprintf("%s/%d/payout", statements, statement)
	transfer := map[string]any{"offline_payment_reference": "offline-fixture", "amount_cents": 200, "offline_paid_at": now}
	f.write("PUT", payout, "merchant", transfer)
	f.write("PUT", payout, "merchant", transfer)
	if got := read[[]settlement.Statement](t, f.request("GET", "/leader/commission-statements", "leader", nil), 200); len(got) != 1 || got[0].ID != statement || got[0].PaidAt == nil {
		t.Fatalf("leader statements: %+v", got)
	}
	if got := read[[]identifier](t, f.request("GET", "/leader/commission-statements", "other-leader", nil), 200); len(got) != 0 {
		t.Fatalf("another leader's statement was exposed: %+v", got)
	}

	transfer["offline_payment_reference"] = "another-transfer"
	read[any](t, f.request("PUT", payout, "merchant", transfer), 409)
	trade := "group-" + strconv.FormatInt(a, 10)
	for _, r := range concurrent(6, func(_ int) reply { return f.refund(a, trade, "partial", 333) }) {
		read[any](t, r, 200)
	}
	f.equal(-34, "SELECT sum(amount_cents) FROM commission_entries WHERE order_id=$1 AND entry_kind='refund_adjustment'", a)
	for _, r := range concurrent(2, func(i int) reply { return f.refund(a, trade, fmt.Sprintf("competing-%d", i), 1000) }) {
		if r.status != 200 && r.status != 409 {
			t.Fatalf("unexpected concurrent refund: %s", r.body)
		}
	}
	f.equal(1333, "SELECT sum(amount_cents) FROM refunds WHERE order_id=$1", a)
	read[any](t, f.refund(a, trade, "remaining", 667), 200)
	f.equal(-200, "SELECT sum(amount_cents) FROM commission_entries WHERE order_id=$1 AND entry_kind='refund_adjustment'", a)
	f.equal(200, "SELECT total_cents FROM commission_statements WHERE id=$1", statement)
	f.equal(2, "SELECT count(*) FROM group_batch_orders WHERE campaign_id=(SELECT campaign_id FROM orders WHERE id=$1)", a)

	code := read[pickupCode](t, f.request("POST", orderPath(b, "/pickup-code"), "buyer", nil), 200)
	read[any](t, f.request("POST", orderPath(b, "/redeem"), "leader", map[string]any{
		"pickup_point_id": f.point, "code": code.Code, "event_id": uuid.NewString()}), 200)
	f.exec("UPDATE commission_entries SET eligible_at=$3 WHERE order_id IN ($1,$2) AND statement_id IS NULL", a, b, start.Add(time.Hour))
	read[any](t, f.request("POST", statements, "merchant", period(start, end)), 409)
	f.equal(4, "SELECT count(*) FROM commission_entries WHERE statement_id IS NULL")

	campaign := f.open(1)
	c := f.write("POST", "/orders", "buyer", f.checkout("next-period", campaign, 1))
	read[any](t, f.request("POST", orderPath(c, "/payment"), "payments", f.receipt("next-period", 1000)), 200)
	f.cutoff(campaign)
	f.write("POST", f.campaign(campaign, "/close"), "merchant", nil)
	f.write("POST", f.campaign(campaign, "/arrival"), "merchant", nil)
	code = read[pickupCode](t, f.request("POST", orderPath(c, "/pickup-code"), "buyer", nil), 200)
	read[any](t, f.request("POST", orderPath(c, "/redeem"), "leader", map[string]any{
		"pickup_point_id": f.point, "code": code.Code, "event_id": uuid.NewString()}), 200)
	f.exec("UPDATE commission_entries SET eligible_at=$2 WHERE order_id=$1", c, start.Add(2*time.Hour))
	next := read[map[string]any](t, f.request("POST", statements, "merchant", period(start, end)), 200)
	if next["total_cents"] != float64(100) {
		t.Fatalf("cross-period offset: %v", next)
	}
	f.equal(0, "SELECT count(*) FROM commission_entries WHERE statement_id IS NULL")
}

func TestPostgresRefundBeforeCutoff(t *testing.T) {
	f := newFixture(t)
	c := f.open(1)
	o := f.write("POST", "/orders", "buyer", f.checkout("precutoff", c, 1))
	read[any](t, f.request("POST", orderPath(o, "/payment"), "payments", f.receipt("precutoff", 1000)), 200)
	read[any](t, f.refund(o, "precutoff", "precutoff-return", 1000), 200)
	f.equal(0, "SELECT paid_quantity FROM group_campaign_skus WHERE campaign_id=$1", c)
	f.equal(20, "SELECT on_hand FROM stock WHERE sku_id=$1", f.sku)
	f.cutoff(c)
	result := read[batch](t, f.request("POST", f.campaign(c, "/close"), "merchant", nil), 200)
	if len(result.Orders) != 0 {
		t.Fatalf("batch includes canceled order: %+v", result)
	}
}

func TestPostgresConcurrentStockAndQuota(t *testing.T) {
	f := newFixture(t)
	f.write("PUT", f.shop(fmt.Sprintf("/skus/%d/stock", f.sku)), "merchant", map[string]int{"on_hand": 1})
	holds := concurrent(2, func(i int) reply {
		return f.request("POST", "/orders", "buyer", f.checkout(fmt.Sprintf("hold-%d", i), 0, 1))
	})
	var held int64
	for _, r := range holds {
		if r.status == 200 {
			if held != 0 {
				t.Fatal("both delivery orders reserved the last unit")
			}
			held = read[identifier](t, r, 200).ID
		} else {
			read[any](t, r, 409)
		}
	}
	if held == 0 {
		t.Fatal("no order reserved the last unit")
	}
	f.equal(1, "SELECT reserved FROM stock WHERE sku_id=$1", f.sku)
	f.write("POST", orderPath(held, "/cancel"), "buyer", nil)
	f.write("PUT", f.shop(fmt.Sprintf("/skus/%d/stock", f.sku)), "merchant", map[string]int{"on_hand": 20})

	c := f.open(1)
	orders := []int64{f.write("POST", "/orders", "buyer", f.checkout("quota-a", c, 1)),
		f.write("POST", "/orders", "buyer", f.checkout("quota-b", c, 1))}
	for _, r := range concurrent(2, func(i int) reply {
		return f.request("POST", orderPath(orders[i], "/payment"), "payments", f.receipt(fmt.Sprintf("quota-%d", i), 1000))
	}) {
		read[any](t, r, 200)
	}
	f.equal(1, "SELECT count(*) FROM payments WHERE disposition='accepted'")
	f.equal(1, "SELECT count(*) FROM payments WHERE disposition='refund_required'")
	f.equal(1, "SELECT paid_quantity FROM group_campaign_skus WHERE campaign_id=$1", c)
	f.equal(19, "SELECT on_hand FROM stock WHERE sku_id=$1", f.sku)
}

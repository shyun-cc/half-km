简体中文 | [English](./README.en.md) | [繁體中文](./README.zh-TW.md)

# 半公里

半公里是一套可獨立部署的開源社區鄰近配送與團購系統。

- 服務圈依樓棟白名單設定配送範圍，由商家自行配送。
- 每筆訂單只屬於一家商店，並使用一種履約方式。團購到時依已付款數量結團，無最低成團門檻。
- 各店使用自有商戶號收款，退款退回原付款方式。系統計算單級團長佣金並登記線下付款。

## 快速啟動

在儲存庫根目錄使用 Docker Compose v2。

```sh
docker compose up --build -d
curl --retry 5 --retry-connrefused -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
docker compose down
```

API 使用 GoFrame v2 與 PostgreSQL 18。Compose 在全新資料卷中依序套用 [migrations](migrations/)，API 繫結本機回送位址，開發資料庫僅可透過 Compose 網路存取。`down` 會保留資料卷。既有實例及獨立資料庫的遷移步驟請參閱[部署指南](docs/deployment.md)。

若不使用 Docker，安裝 Go 1.25.7 以上版本後執行 `go run ./apps/api`。使用 `HTTP_ADDR` 設定監聽位址，使用 `DATABASE_URL` 連線至 PostgreSQL，使用 `HALFKM_AUTH_FILE` 設定業務角色。

## 里程碑 2

即時配送支援樓棟覆蓋、庫存鎖定、優惠券、收款入帳及商家送達。團購支援依已付款數量自動結團、分揀、取貨核銷、退款登記及團長佣金線下結算。買家與商家介面及正式金流串接安排在後續版本。

提交前執行 `gofmt -l apps/api`、`go vet ./...` 和 `go test ./...`。將 `HALFKM_TEST_DATABASE_URL` 指向隔離的 PostgreSQL 18 資料庫即可執行 HTTP 交易及並行測試。GitHub Actions 會啟動專用測試資料庫。

[使用手冊（簡體中文）](docs/index.md)

## 授權

[AGPL-3.0](LICENSE)。

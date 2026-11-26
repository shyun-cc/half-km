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

API 使用 GoFrame v2 與 PostgreSQL 18。Compose 在全新資料卷中初始化資料庫結構，API 僅繫結本機回送位址，開發資料庫僅可透過 Compose 網路存取。`down` 會保留資料卷。若使用獨立空資料庫，透過一般連線設定執行一次 `psql -v ON_ERROR_STOP=1 -f migrations/0001_initial.sql` 套用[初始遷移](migrations/0001_initial.sql)。

若不使用 Docker，安裝 Go 1.25.7 以上版本後執行 `go run ./apps/api`，預設監聽 8080。可透過 `HTTP_ADDR` 修改監聽位址，透過 `DATABASE_URL` 啟用資料庫就緒檢查。

## 里程碑 1

目前交付可執行的健康檢查 API、PostgreSQL 資料結構，以及付款重送、結團與取貨核銷的領域測試。買家與商家介面、交易流程持久化及正式金流串接安排在後續里程碑。

提交前執行 `gofmt -l apps/api`、`go vet ./...` 和 `go test ./...`。

[使用手冊（簡體中文）](docs/index.md)

## 授權

[AGPL-3.0](LICENSE)。

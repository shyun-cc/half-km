简体中文 | [English](./README.en.md) | [繁體中文](./README.zh-TW.md)

# 半公里

半公里是一套可独立部署的开源社区附近配送与团购系统。

- 服务圈按楼栋白名单配置覆盖范围，由商户自行配送。
- 一单一店、一种履约方式。团购到时按已付数量截团，无最低成团门槛。
- 各店通过自有收款商户号收款、原路退款。系统计算单级团长佣金并登记线下付款。

## 快速启动

在仓库根目录使用 Docker Compose v2。

```sh
docker compose up --build -d
curl --retry 5 --retry-connrefused -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
docker compose down
```

API 使用 GoFrame v2 与 PostgreSQL 18。Compose 在全新数据卷初始化数据库结构，API 仅绑定本机回环地址，开发数据库仅在 Compose 网络内可用。`down` 会保留数据卷。如使用独立的空库，通过常规连接配置执行一次 `psql -v ON_ERROR_STOP=1 -f migrations/0001_initial.sql` 应用[初始迁移](migrations/0001_initial.sql)。

不使用 Docker 时，安装 Go 1.25.7 或更高版本，运行 `go run ./apps/api`，默认监听 8080。可通过 `HTTP_ADDR` 修改监听地址，通过 `DATABASE_URL` 启用数据库就绪检查。

## 里程碑 1

当前交付可运行的健康检查 API、PostgreSQL 数据结构和支付重放、截团、提货核销的领域测试。买家与商户界面、业务流程持久化和真实支付渠道接入安排在后续里程碑。

提交前运行 `gofmt -l apps/api`、`go vet ./...` 和 `go test ./...`。

[用户使用手册](docs/index.md)

## 许可证

[AGPL-3.0](LICENSE)。

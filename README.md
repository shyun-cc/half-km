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

API 使用 GoFrame v2 与 PostgreSQL 18。Compose 在全新数据卷中依序应用 [migrations](migrations/)，API 绑定本机回环地址，开发数据库仅在 Compose 网络内可用。`down` 保留数据卷。现有实例和独立数据库的迁移步骤见[部署指南](docs/deployment.md)。

不使用 Docker 时，安装 Go 1.25.7 或更高版本并运行 `go run ./apps/api`。使用 `HTTP_ADDR` 设置监听地址，使用 `DATABASE_URL` 连接 PostgreSQL，使用 `HALFKM_AUTH_FILE` 配置业务角色。

## 里程碑 2

即时配送支持楼栋覆盖、库存锁定、优惠券、收款入账和商户送达。团购支持按已付数量自动截团、分拣、自提核销、退款登记和团长佣金线下结算。买家与商户界面及真实支付渠道集成安排在后续版本。

提交前运行 `gofmt -l apps/api`、`go vet ./...` 和 `go test ./...`。将 `HALFKM_TEST_DATABASE_URL` 指向隔离的 PostgreSQL 18 数据库可运行 HTTP 事务与并发测试。GitHub Actions 会启动专用测试数据库。

[用户使用手册](docs/index.md)

## 许可证

[AGPL-3.0](LICENSE)。

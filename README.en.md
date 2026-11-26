简体中文 | [English](./README.en.md) | [繁體中文](./README.zh-TW.md)

# Half Kilometer

Half Kilometer is a standalone, open-source system for neighborhood delivery and community group buying.

- Stores deliver their own orders to buildings on an explicitly configured service-circle allowlist.
- Each order belongs to one store and uses one fulfillment method. At the cutoff, a group purchase closes with the quantities paid in time, regardless of volume.
- Each store uses its own merchant account for collections and refunds. The system calculates single-level group leader commissions and records offline payouts.

## Quick start

From the repository root, run Docker Compose v2.

```sh
docker compose up --build -d
curl --retry 5 --retry-connrefused -fsS http://127.0.0.1:8080/health
curl -fsS http://127.0.0.1:8080/ready
docker compose down
```

The API uses GoFrame v2 and PostgreSQL 18. Compose applies the [migrations](migrations/) in order on a fresh volume, publishes the API on loopback, and keeps the development database on the Compose network. `down` preserves the volume. See the [deployment guide](docs/deployment.md) for existing instances and standalone databases.

To run without Docker, install Go 1.25.7 or later and run `go run ./apps/api`. Set `HTTP_ADDR` for the bind address, `DATABASE_URL` for PostgreSQL, and `HALFKM_AUTH_FILE` to configure business roles.

## Milestone 2

Neighborhood delivery supports building coverage, stock reservations, coupons, receipt recording, and merchant delivery. Group buying supports automatic cutoff by paid quantity, sorting, pickup verification, refund recording, and offline leader payouts. Buyer and store interfaces and live payment-provider integrations are planned for later releases.

Run `gofmt -l apps/api`, `go vet ./...`, and `go test ./...` before submitting changes. Point `HALFKM_TEST_DATABASE_URL` at a disposable PostgreSQL 18 database to run the HTTP transaction and concurrency tests. GitHub Actions starts a dedicated test database.

[User guide (Simplified Chinese)](docs/index.md)

## License

[AGPL-3.0](LICENSE).

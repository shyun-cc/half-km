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

The API uses GoFrame v2 and PostgreSQL 18. Compose initializes the schema in a fresh database volume, publishes the API on loopback, and keeps the development database on the Compose network. `down` preserves the data volume. For a separate empty database, apply the [initial migration](migrations/0001_initial.sql) once using `psql -v ON_ERROR_STOP=1 -f migrations/0001_initial.sql` with your normal connection settings.

To run the API without Docker, install Go 1.25.7 or later and run `go run ./apps/api`. It listens on port 8080 by default. Set `HTTP_ADDR` to override the bind address and `DATABASE_URL` to enable the database readiness endpoint.

## Milestone 1

The repository ships a runnable health API, a PostgreSQL schema and domain tests for payment replay, group cutoff and pickup redemption. Buyer and store interfaces, workflow persistence and real payment-provider integration are future milestones.

Run `gofmt -l apps/api`, `go vet ./...` and `go test ./...` before submitting changes.

[User guide (Simplified Chinese)](docs/index.md)

## License

[AGPL-3.0](LICENSE).

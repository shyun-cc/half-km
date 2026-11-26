# Working in Half Kilometer

- Keep `docs/` for the public user manual. Authorized maintainers keep internal requirements, architecture and decisions in the private Gitea repository `shw-documents/half-km`. The repository ships a standalone neighborhood delivery and group-buying system.
- Start locally with `docker compose up --build -d` or `go run ./apps/api`. Run `gofmt -l apps/api`, `go vet ./...` and `go test ./...` before submitting changes.
- Keep module writes behind their owning application interfaces. When adding persistence to a workflow, enforce the invariants within one PostgreSQL transaction and use unique keys for replay protection.
- Use integer cents and basis points. Preserve order, payment account and commission snapshots. Keep the campaign cutoff exclusive and serialize group payment acceptance and closing on the same campaign.
- Keep migrations ordered and schema-only. Use a new migration after a version has been deployed.
- Use public dependencies and container images. Keep credentials, internal service addresses, real customer data and database dumps out of the repository. Run a redacted Gitleaks scan before publishing changes.
- Update all three READMEs together when their shared instructions change. Preserve the AGPL-3.0 license.

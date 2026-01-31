# Logryph

Logryph records AI agent tool calls.
It sits in front of a tool server as an HTTP reverse proxy.
It writes each request and response to a SQLite database.
Each record is hashed and signed so changes are detectable.
You can inspect, verify, and export records with a CLI.

## Quick start

Build:
```bash
go build -o logryph main.go
go build -o logryph-cli cmd/logryph-cli/main.go
```

Run:
```bash
./logryph --target http://localhost:8080 --port 9999 --backpressure drop
```

Use:
```bash
./logryph-cli trace
./logryph-cli verify
./logryph-cli export <file.zip>
```

Ports: proxy `:9999`, admin/metrics `:9998`

## Files

- Config: `logryph-policy.yaml`
- Database: `logryph.db`
- Key: `.logryph_key`
- Schema: `internal/ledger/store/schema.sql`

## Demo (optional)

Run a small local demo:
```bash
go run examples/scenario/server/main.go
./logryph --config examples/scenario/policy.yaml
go run examples/scenario/agent/main.go
```

Then inspect:
```bash
./logryph-cli trace
./logryph-cli verify --skip-live
```

The demo writes `logryph.db` and `.logryph_key` to the working directory.

## Docs

- ARCHITECTURE.md
- KEY_MANAGEMENT.md
- CONTRIBUTING.md

## License

Apache 2.0

# Logryph

Logryph records tool calls made by an AI agent.
It sits between the agent and the tool server.
It forwards traffic and saves a copy of each request and response.
Records are stored in SQLite.
Each record is hashed and signed so edits can be detected.
You can inspect, verify, and export records with a CLI.

## Why it exists

When agents use tools, you often need a reliable record of what happened.
Logryph creates that record so you can review, verify, and export it.

## What it does

- Records every tool request and response
- Stores data in `logryph.db`
- Chains records with hashes
- Signs records with a local key
- Lets you query and export with `logyctl`

## Quick start

Build:
```bash
go build -o logryph main.go
go build -o logyctl cmd/logyctl/main.go
```

Run:
```bash
./logryph --target http://localhost:8080 --port 9999 --backpressure drop
```

Send your agent traffic to:
```
http://localhost:9999
```

Use the CLI:
```bash
./logyctl trace
./logyctl verify
./logyctl export <file.zip>
```

Ports: proxy `:9999`, admin/metrics `:9998`

Backpressure:
- `drop` keeps requests fast but can lose records under load
- `block` slows requests to keep all records

## Usage

Server flags:
```bash
./logryph --config logryph-policy.yaml --target http://localhost:8080 --port 9999 --backpressure drop
```

- `--config` — path to the policy file
- `--target` — tool server URL
- `--port` — proxy listen port
- `--backpressure` — `drop` or `block`

CLI commands:

- `logyctl status` — show current run info
- `logyctl events --limit 10` — list recent events
- `logyctl stats` — show run and global stats
- `logyctl risk` — list high‑risk events
- `logyctl trace <task-id>` — show a task timeline
- `logyctl verify` — verify the hash chain
- `logyctl verify --skip-live` — verify without live Bitcoin checks
- `logyctl export <file.zip>` — export an evidence bag
- `logyctl replay <event-id>` — replay a stored tool call
- `logyctl rekey` — rotate signing keys
- `logyctl backup-key` — save a key backup
- `logyctl restore-key <backup-file>` — restore from a backup
- `logyctl list-backups` — list available backups

## Environment

- `LOGRYPH_ADMIN_TOKEN` protects the admin rekey endpoint
- `LOGRYPH_LOG_LEVEL` controls log verbosity

## Files

- Config: `logryph-policy.yaml`
- Database: `logryph.db`
- Key: `.logryph_key`
- Schema: `internal/ledger/store/schema.sql`

## Docs

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [KEY_MANAGEMENT.md](KEY_MANAGEMENT.md)
- [CONTRIBUTING.md](CONTRIBUTING.md)
- [ROADMAP.md](ROADMAP.md)

## License

Apache 2.0

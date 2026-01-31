# Logryph: The AI Agent Flight Recorder

> **"If it isnt in the ledger, it didnt happen."**

hit the star if you like the repo ⭐️

Logryph is a **forensic-grade flight recorder** for autonomous AI agents. It passively captures tool execution, cryptographically signs every action, and maintains an immutable, tamper-evident audit trail.

---

## Quick Start

### 1. Build
```bash
go build -o logryph main.go
go build -o logryph-cli cmd/logryph-cli/main.go
```

### 2. Start Recording
```bash
./logryph --target http://localhost:8080 --port 9999 --backpressure drop
```

Backpressure strategies:
- `drop` (default): fail-open, events are dropped when the buffer is full
- `block`: fail-closed, requests block until buffer space is available

### 3. Investigate
```bash
./logryph-cli trace    # Reconstruct timelines
./logryph-cli verify   # Prove integrity
./logryph-cli export   # Sealed evidence bag
```

---

## Why Logryph?

*   **Immutable**: SQLite ledger with SHA-256 chaining. If a single byte is altered, verification fails.
*   **Cryptographic Proof**: Every event is signed with an internal Ed25519 key—proving the record came from Logryph.
*   **Forensic Ready**: Meets [FRE 902(13)](https://www.law.cornell.edu/rules/fre/rule_902) standards for self-authenticating electronic records.
*   **Bitcoin Anchored**: Genesis blocks and periodic state are anchored to the Bitcoin blockchain for external proof-of-existence.
*   **Dynamic Policies**: Hot-reload security rules from `logryph-policy.yaml` without restarting the server.
*   **Production Ready**: Prometheus metrics endpoint with queue depth and latency histograms.
*   **High Performance**: < 2ms overhead with zero-allocation memory pools.

---

## Documentation

You do **not** need all docs to use Logryph. If you only want to record and inspect events locally, the Quick Start above is enough. The guides below are optional and scoped to specific audiences:

- **[ARCHITECTURE.md](ARCHITECTURE.md)**: System design, diagrams, and packet flow (for contributors and reviewers).
- **[INVESTIGATOR_GUIDE.md](INVESTIGATOR_GUIDE.md)**: Incident response workflow (for security/forensics teams).
- **[CLOUD_DEPLOYMENT.md](CLOUD_DEPLOYMENT.md)**: Docker and production ops (for SRE/DevOps).
- **[CONTRIBUTING.md](CONTRIBUTING.md)**: Development workflow and safety standards (for contributors).
- **[Examples](examples/scenario/README.md)**: Live "Rogue Agent" investigation scenario (for demos).

---

## License
Apache 2.0

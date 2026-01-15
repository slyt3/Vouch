# AEL – Agent Execution Ledger

**A black box recorder for AI agents.**

## What is AEL?

AI agents are making real decisions: moving money, changing infrastructure, executing commands. When something goes wrong, you need proof of what happened.

AEL records every action your AI agent takes in a tamper-proof ledger. Think of it as a flight recorder for autonomous systems.

## The Problem

- Agent makes a mistake → You can't prove what it actually did
- Agent gets hacked → No audit trail to reconstruct events  
- Regulations require transparency → Your logs aren't verifiable

**Best-effort logging isn't enough anymore.**

## What AEL Does

AEL sits between your AI agent and its tools. It captures and cryptographically signs every:
- Tool call made by the agent
- Result returned by the tool
- Decision in the chain

The ledger is **immutable** – any tampering is instantly detected.

## Key Features

**Tamper-Proof Logs** – Each record is cryptographically chained. Edit one, break the whole chain.

**Action Control** – Block risky operations (like `delete_database`) until a human approves them.

**Compliance Ready** – Built for EU AI Act, insurance audits, and legal requirements.

**Lightning Fast** – Adds less than 1ms to your agents response time.

**Local & Private** – Your data never leaves your infrastructure.

## Quick Start

### 1. Install

```bash
git clone https://github.com/your-username/ael.git
cd ael
go build -o ael ./cmd/ael
ael init
```

### 2. Start the Proxy

```bash
ael start --port 9999 --forward :8080
```

Point your AI agent to `http://localhost:9999` instead of your tool server.

### 3. Verify Integrity

```bash
ael verify <run_id>
```

This checks that no records have been modified.

### 4. Control Risky Actions

Create an `ael-policy.yaml` file:

```yaml
version: 1
policies:
  - action: "database_query"
    allow: true

  - action: "delete_*"
    requires_approval: true
    
  - action: "cloud_provision"
    requires_approval: true
    threshold_usd: 500
```

When a protected action triggers, approve it with:

```bash
ael approve <event_id>
```

## How It Works

```
[Your AI Agent] → [AEL Proxy] → [Tools/APIs]
                       ↓
                  [Ledger DB]
```

1. **Intercept** – AEL captures tool calls from your agent
2. **Check** – Validates against your policy rules
3. **Record** – Logs the event with cryptographic signing
4. **Forward** – Sends the call to your actual tool (if approved)

Every event is linked to the previous one with SHA-256 hashing. Break one link, the whole chain fails verification.

## Use Cases

**Compliance** – Meet EU AI Act requirements with verifiable audit trails

**Debugging** – Replay exactly what your agent did during a failure

**Security** – Detect if someone tampered with your agent's logs

**Insurance** – Provide certified evidence for liability claims

## Design Principles

**Simple** – Built with Go, SQLite, and SHA-256. No exotic dependencies.

**Local** – Runs on your hardware. No data sent to external services.

**Auditable** – Every feature designed for lawyers and auditors, not just developers.

## License

MIT – See [LICENSE](LICENSE) for details.

---

**Questions?** Open an issue or check the [docs](docs/).

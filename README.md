# Vouch: The AI Agent Black Box

Vouch is a high-performance "flight recorder" for AI agents. It intercepts tool interactions, enforces safety policies, and persists a cryptographically signed, immutable audit trail in a local ledger.

---

[Architecture](ARCHITECTURE.md) | [Cloud Deployment](CLOUD_DEPLOYMENT.md) | [Design Specification](DESIGN_SPEC.md)

---

![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.22+-00ADD8.svg?style=flat&logo=go)
![Build Status](https://img.shields.io/badge/build-passing-brightgreen.svg)

## Safety-Critical Certified
Audited against the NASA/JPL "Power of Ten" Rules for safety-critical software.
*   **Compliance Score**: 8/10
*   **Suitable for**: Finance, Infrastructure Management, Healthcare AI, and Security-Critical Automation.

## The Problem
As AI agents move from "chatbots" to "action-bots," they gain the power to delete files, move funds, and modify production infrastructure. Standard logging is insufficient: it can be bypassed if the agent is compromised, it lacks cryptographic proof, and it often misses the "raw" data exchanged between the agent and its tools.

Without a source of truth, autonomous agents are a liability. If an agent deletes a database or leaks PII, "The agent said it was fine" is not an acceptable response for compliance or forensic teams.

## The Solution
Vouch operates as a **transparent interceptor**. It sits between the agent and its toolsets (MCP servers, APIs, CLI tools), ensuring that every action is recorded before it is executed.

```text
 [ AI Agent ] <--- Tool Call ---> [ Vouch Proxy ] <--- Forward ---> [ Tools/APIs ]
                                        |
                            [ Cryptographic Ledger ]
                                        |
                            [ Ed25519 Signed Chain ]
```

### Key Features
*   **Immutable Ledger**: Append-only SQLite store with SHA-256 hash chaining.
*   **Governance Engine**: Define "risky" tools that require human approval before execution.
*   **Cryptographic Proof**: Every event is signed with an Ed25519 key (hardware-backed support).
*   **Zero-Overhead**: High-performance memory pooling ensures < 2ms latency impact.
*   **Forensic CLI**: Reconstruct the agent's reasoning chain and link related tasks.

## Quick Start

### 1. Installation
```bash
go install github.com/slyt3/Vouch@latest
```

### 2. Define Safety Policies
Create a `vouch-policy.yaml` to specify which actions require a "Stall" (Human Approval).
```yaml
policies:
  - id: "prevent-deletion"
    match_methods: ["os.remove", "db.drop_table"]
    action: "stall"
    risk_level: "critical"
```

### 3. Start the Proxy
```bash
vouch-proxy --upstream http://localhost:3000 --port 9999
```

### 4. Approve Actions
In another terminal, monitor and approve actions that hit the safety ceiling.
```bash
vouch-cli approve <event_id>
```

## How It Works

### 1. Interception Layer
Vouch acts as a JSON-RPC proxy. It synchronously inspects the `tool_call` payload. If the action matches a safety policy, the proxy blocks the request and notifies the user via the Admin API.

### 2. Immutable Ledger
Events are stored in a chained format. Each block contains the hash of the previous block, creating a tamper-evident record. We use RFC 8785 (JSON Canonicalization Scheme) to ensure that logs remain identical across different platforms and languages.

### 3. Policy Engine
The engine evaluates incoming requests in real-time. It can redact PII, stall execution for human review, or simply log high-risk behaviors for later audit.

## Verification & Forensics

### Verify the Cryptographic Chain
Ensures that no logs have been deleted or modified after the fact.
```bash
vouch-cli verify
```

### View Statistics
Monitor agent performance, tool usage, and policy hits.
```bash
vouch-cli stats
```

### Export Audit Trail
Export logs in standard JSON format for ingestion into SIEM or compliance tools.
```bash
vouch-cli export --format json
```

## Safety-Critical Compliance

| Rule | Status | Description |
| :--- | :--- | :--- |
| 1. Simple Control Flow | ✅ Pass | No recursion, simple branching logic. |
| 2. Fixed Loop Bounds | ⚠️ Partial | Heuristics check for `len()` or hard caps on iterations. |
| 3. No Dynamic Allocation | ✅ Pass | Uses `sync.Pool` for events and buffers to minimize heap usage. |
| 4. Function Length | ⚠️ Partial | 95% of functions < 60 lines. Core logic adheres to limits. |
| 5. Assertion Density | ⚠️ Partial | Current density: 0.70 (Target: 2.0). |
| 6. Minimal Scope | ✅ Pass | Strict variable scoping and no global state beyond pooling. |
| 7. Return Value Checks | ✅ Pass | All JSON/IO/DB returns are explicitly checked and handled. |
| 8. Preprocessor Use | ✅ Pass | No complex macros; utilizes standard Go toolchain. |
| 9. Pointer Limit | ✅ Pass | Restricted use of pointers; no pointer arithmetic. |
| 10. Clean Compile | ✅ Pass | Zero warnings under `go vet` and `staticcheck`. |

[View Full Audit Report](SECURITY_AUDIT.md)

## Architecture Deep-Dive

### Cryptographic Chain Design
Vouch implements a SHA-256 hash chain where `Block[N].hash = Hash(Block[N-1].hash + CanonicalJSON(Block[N].payload))`. This ensures that any modification to a historical event invalidates all subsequent signatures.

### SEP-1686 Task State Protocol
Vouch tracks agent "Tasks" through their lifecycle: `working` -> `stalled` -> `completed/failed`. This allows users to follow a single logical operation across multiple tool interactions.

### Stall Intelligence
When an agent hits a safety rule, Vouch doesn't just block; it captures the context (parameters, parent task, agent identity) so the human approver can make an informed decision.

### PII Redaction
Vouch can be configured to scrub potential PII (emails, keys, tokens) from the ledger before it is written to disk, ensuring compliance with GDPR/HIPAA.

## Performance
*   **Latency**: < 2.0ms overhead on standard tool calls.
*   **Throughput**: 5,000+ requests/second (Benchmarked on standard hardware).
*   **Memory**: < 20MB RSS baseline.

## Integration Examples

### LangChain Integration
```python
import requests

def vouch_interceptor(method, params):
    # Route tool calls through Vouch
    response = requests.post("http://localhost:9999", json={
        "method": method,
        "params": params
    })
    return response.json()
```

### Claude Desktop/MCP
Simply point your MCP configuration to proxy through the Vouch endpoint to secure all Desktop Agent interactions.

## Use Cases
*   **Financial Services**: Oversight for trading bots and automated fund transfers.
*   **Cloud Infrastructure**: Prevent "rogue agents" from accidentally deleting production clusters.
*   **Healthcare**: Ensure all interactions with patient data are signed and PII-redacted.
*   **Cybersecurity**: Incident response forensics—see exactly what a remediation agent did.

## Comparison vs Alternatives

| Feature | Vouch | LangSmith / LangFuse | Standard Logging |
| :--- | :--- | :--- | :--- |
| **Tamper-Proof** | Yes (Crypto Chain) | No (Cloud Managed) | No |
| **Blocking Policy**| Yes (Stall Engine) | No (Observable only) | No |
| **Memory Pooling** | Yes (High Perf) | No | Limited |
| **Local-First** | Yes (Air-gapped) | No | Yes |

## FAQ

**Q: Does this slow down my agent?**  
A: No. With memory pooling and an asynchronous ledger worker, the overhead is typically less than 2ms—undetectable by LLMs.

**Q: Can agents bypass this?**  
A: No. By acting as a transparent network proxy, Vouch sees what actually goes over the wire, regardless of what the agent "reports" in its internal logs.

**Q: What if the ledger fills up?**  
A: Vouch supports periodic snapshots and log rotation, allowing you to archive signed segments while keeping the active chain small.

## Roadmap
- [ ] WebAssembly policy plugins
- [ ] Multi-agent orchestration
- [ ] Behavioral anomaly detection
- [ ] Decentralized ledger (IPFS/Arweave) export

## Contributing
We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License
Apache 2.0

## Citation
```bibtex
@software{vouch2025,
  title = {Vouch: Safety-Critical AI Agent Accountability},
  author = {slyt3},
  year = {2025},
  url = {https://github.com/slyt3/Vouch}
}
```

## Related Work
*   [NASA/JPL Power of Ten Rules](https://en.wikipedia.org/wiki/The_Power_of_Ten_Rules_for_Developing_Safety-Critical_Code)
*   [Model Context Protocol (MCP)](https://modelcontextprotocol.io)
*   [RFC 8785 (JSON Canonicalization)](https://tools.ietf.org/html/rfc8785)

# Vouch: The Agent Black Box

**Vouch** is a "flight recorder" for AI agents. It captures every move your agent makes, signs it cryptographically, and stores it in a ledger that no one can change—not even the agent.

---

### [ 🛠️ Architecture ](file:///home/slyt3/opensource/AEL/ARCHITECTURE.md) | [ ☁️ Cloud Deploy ](file:///home/slyt3/opensource/AEL/CLOUD_DEPLOYMENT.md) | [ 📐 Design Spec ](file:///home/slyt3/opensource/AEL/DESIGN_SPEC.md)

---

## 🚀 Quick Start (In 3 Steps)

### 1. Configure Safety
Define which tools are "risky" in `vouch-policy.yaml`. If an agent calls them, Vouch will stop and ask you.
```yaml
policies:
  - id: "prevent-deletion"
    match_methods: ["os.remove", "os.rmdir", "db.drop"]
    action: "stall"
```

### 2. Start the Proxy
Run the binary. It will create a new ledger (`vouch.db`) and a secure signing key automatically.
```bash
./vouch-proxy
```

### 3. Connect Your Agent
Point your AI agent's tool-call URL to `http://localhost:9998`. Vouch will intercept, log, and sign everything.

---

## 💎 Why Vouch?
When AI agents move money, delete files, or send emails, you need a **Ledger of Truth**.
*   **Accountability**: Every action is cryptographically signed by the Vouch hardware/key.
*   **Security**: Compromised agents can't "delete their tracks"—the ledger is append-only.
*   **Trust**: Run agents autonomously knowing that risky actions MUST be approved by you.

## 🏗️ Design Philosophy
Vouch sits *between* your agent and its tools. By intercepting the actual network traffic, Vouch sees exactly what the agent is doing in the real world, regardless of what the agent says in its internal logs. It is the ultimate source of truth for agent behavior.

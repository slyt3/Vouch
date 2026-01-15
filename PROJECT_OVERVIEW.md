# Vouch: The Agent Black Box - Project Overview

## 1. Executive Summary
**Vouch** is a specialized security and accountability layer designed for AI Agents. It serves as a "Flight Recorder" or "Black Box," providing a tamper-proof record of every action an agent takes in the real world. By operating as a transparent proxy, Vouch ensures that agent behavior is recorded, verified, and governed without requiring any changes to the agent's core logic.

## 2. Core Vision
AI agents are increasingly autonomous, moving money, managing infrastructure, and accessing sensitive data. Vouch solves the "Trust Gap" by providing:
*   **Irrefutable Evidence**: Cryptographically signed logs that prove exactly what happened.
*   **Safety Governance**: Real-time policy enforcement that stalls risky actions for human approval.
*   **Operational Intelligence**: Analytics on agent performance, failure rates, and tool usage hierarchy.

## 3. High-Level Architecture
Vouch is built in Go for maximum performance and follows a non-invasive proxy pattern.

### Interception Layer (The Interceptor)
Vouch sits between the Agent and the Model Context Protocol (MCP) tools. It intercepts JSON-RPC traffic, allowing it to:
*   Log every tool request and response.
*   Enforce policies before a request reaches the tool.
*   Inject metadata (like `task_id`) for better tracking.

### Immutable Ledger (The Vault)
Every event is stored in a SHA-256 hash-chain (similar to a blockchain approach):
*   **Hashing**: Each event includes the hash of the previous event, creating a continuous chain.
*   **Signing**: Each event is signed using Ed25519 private keys.
*   **Persistence**: Uses SQLite in WAL (Write-Ahead Logging) mode for high-concurrency and reliability.

### Governance System (The Guard)
A YAML-based policy engine (`vouch-policy.yaml`) defines how Vouch should handle specific tool calls:
*   **Actions**: `allow`, `stall` (wait for human), or `redact`.
*   **Wildcards**: Support for broad matching (e.g., `aws:s3:*`).
*   **Human-in-the-Loop**: A CLI tool allows administrators to approve or reject stalled actions.

## 4. Key Security Features

### Stall Intelligence
Vouch tracks the failure history of specific tasks. If an agent tries to use a risky tool on a task that has already failed multiple times, Vouch can provide extra warnings or automatically stall the action, preventing "hallucination loops."

### PII Redaction & Log Scrubbing
Vouch can be configured to redact sensitive fields (like API keys, passwords, or customer data) before they ever reach the persistent ledger, ensuring compliance with data privacy regulations.

### Tamper-Proofing
The cryptographic chain ensures that even if an agent or an attacker gains access to the database, they cannot modify or delete previous logs without breaking the signature chain, making the history irrefutable.

## 5. Components
*   **`vouch-proxy`**: The main server that intercepts traffic and manages the worker.
*   **`vouch-cli`**: The administrative tool for verifying the chain, viewing stats, and approving actions.
*   **The Ledger**: A local SQLite database (`vouch.db`) and signing keys (`.vouch_key`).

## 6. Project Roadmap
Vouch is developed in phases:
*   **Phase 1-3**: Core proxy, immutable ledger, and policy guard (Completed).
*   **Phase 4**: Advanced forensics, stall intelligence, and PII redaction (Completed).
*   **Phase 5**: Continuous Integration and Production Hardening (Completed).
*   **Future**: Multi-agent orchestration, decentralized ledger options, and advanced behavioral anomaly detection.

---
*Vouch: Accountability for the Autonomous Era.*

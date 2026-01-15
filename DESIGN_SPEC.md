# Vouch Design Specification

This document details the core design philosophies and technical specifications of Vouch.

## The "Black Box" Concept

Inspired by flight data recorders (FDRs) used in aviation, Vouch is designed to be:
1.  **Independent**: Not part of the agent's logic.
2.  **Robust**: Survives agent crashes or compromises.
3.  **Traceable**: Provides a "breadcrumb trail" of every tool interaction.

## Hashing & Integrity Protocol

Vouch uses a strict cryptographic chain to prevent tampering.

### 1. Canonicalization (RFC 8758)
To ensure the same payload always hashes to the same value across different JSON libraries, Vouch uses the **JSON Canonicalization Scheme (JCS)**.
*   Keys are sorted alphabetically.
*   Whitespace is removed.
*   Escaping is standardized.

### 2. Block Structure
Every event (or "block") contains:
*   `prev_hash`: The SHA-256 of the previous block.
*   `payload`: The event data (method, params, timestamp).
*   `current_hash`: `SHA-256(prev_hash + JCS(payload))`.
*   `signature`: `Ed25519_Sign(current_hash)`.

## Task State Protocol (SEP-1686)

Vouch implements **SEP-1686** to track complex, multi-step agent behaviors.

| State | Transition Logic |
| :--- | :--- |
| **working** | Default state when a `tool_call` starts. |
| **stalled** | Action blocked by policy, waiting for human approval. |
| **completed** | Task finished successfully; terminal state. |
| **failed** | Task terminated with error; terminal state. |
| **cancelled** | User or system aborted task; terminal state. |

## Forensic Visualization

The Vouch CLI uses the ledger to reconstruct agent "Topologies". By linking `parent_id` and `task_id`, Vouch can map exactly how one tool call led to another, exposing the agent's reasoning chain.

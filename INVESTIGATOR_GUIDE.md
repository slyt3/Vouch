# Investigator's Guide to Vouch

This guide provides procedures for performing forensic investigations using the Vouch Associated Evidence Ledger (AEL).

## 1. Establishing Chain of Custody
Before analyzing agent actions, you must prove the audit trail hasn't been tampered with.

```bash
# Verify the entire ledger integrity
./vouch-cli verify
```
**Verification Checks:**
1.  **Merkle Linkage**: Ensures no events were deleted or inserted in the past.
2.  **Ed25519 Authenticity**: Ensures all records were signed by the authorized Vouch instance.
3.  **Bitcoin Anchoring (Live)**: Cross-references both the genesis and periodic terminal hashes against the public Bitcoin blockchain via the Blockstream API.

## 2. Reconstructing Incidents
When a specific failure occurs, use the Task ID provided by the agent or extracted from high-risk logs.

### A. Surface High-Risk Actions
```bash
./vouch-cli risk
```
Look for `critical` or `high` risk tags associated with deletion, financial transactions, or unauthorized access.

### B. Visualize Causal Timelines
Reconstruct the agent's thought process and tool dependency tree.
```bash
./vouch-cli trace <task-id>
# Or generate a report for stakeholders
./vouch-cli trace <task-id> --html report.html
```

## 3. Incident Reproduction (Replay)
To verify if a bug is reproducible or to test a safety fix, replay the original request.

```bash
./vouch-cli replay <event-id> --target http://localhost:8080
```
This re-sends the exact parameters stored in the ledger and compares the new output with the original response recorded during the incident.

## 4. Evidence Packaging
For legal handover or external compliance audits, package the run into a cryptographically sealed Evidence Bag.

```bash
./vouch-cli export evidence_bag.zip
```
The ZIP contains:
*   `vouch.db`: The raw, immutable SQLite ledger.
*   `manifest.json`: Metadata including the terminal chain hash and export timestamp.

---

## Technical Appendix: Failure Modes
*   **Tamper Detected**: If `./vouch-cli verify` fails, the ledger is compromised. Do not use for legal proceedings without manual binary forensic analysis of the `prev_hash` chain.
*   **Gap Detected**: Indicates Vouch dropped events due to extreme load (fail-open mode). Check system metrics for `ring_buffer_full` events.

# Logryph Forensic Scenario: The Rogue Agent

This example demonstrates how Logryph acts as a "Black Box" flight recorder when an AI agent performs unauthorized or high-risk actions.

## The Situation
An AI agent assigned to "Infrastructure Audit" incorrectly identifies a production database (`prod-users-v2`) as "legacy" and attempts to delete it. Logryph records every nuance of this interaction, categorizes the risk, and provides a tamper-proof audit trail for investigators.

## Prerequisites
1. Build Logryph and the CLI:
   ```bash
   go build -o logryph main.go
   go build -o logryph-cli cmd/logryph-cli/main.go
   ```

## Step 1: Start the Environment
Run these in separate terminal windows:

1. **Start the Mock Cloud API**:
   ```bash
   go run examples/scenario/server/main.go
   ```

2. **Start Logryph (The Recorder)** with scenario policies:
   ```bash
   ./logryph --config examples/scenario/policy.yaml
   ```

## Step 2: Run the Rogue Agent
Execute the agent task:
```bash
go run examples/scenario/agent/main.go
```

The agent will list instances (Low Risk) and then attempt the deletion (Critical Risk).

## Step 3: Forensic Investigation
Now, assume the role of a security investigator. Use `logryph-cli` to inspect the "Black Box".

### 1. View the Timeline
See exactly what happened and when:
```bash
./logryph-cli trace
```
*Look for the '[!!]' symbol marking the critical deletion attempt.*

### 2. Verify Evidence Integrity
Prove that the logs haven't been tampered with:
```bash
./logryph-cli verify
```

### 3. Export Evidence Bag
Pack everything for legal review:
```bash
./logryph-cli export evidence.zip
```

### 4. Risk Analytics
See the high-level impact across this session:
```bash
./logryph-cli stats
```

## Summary
Even if the agent "hides" its logs or the API server is wiped, Logryph's tamper-proof ledger (anchored to Bitcoin) provides an unalterable record of truth for forensic analysis.

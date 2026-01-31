# Logryph Roadmap (Detailed)

Status legend:
- Done: completed and verified
- In progress: actively being worked on
- Planned: approved but not started
- Backlog: not scheduled yet

---

## v0.1.0


## Foundation and MVP hardening

1) Release‑ready build and run
- Status: Done
- Scope: single binary build, quick start run path, default policy loading
- Acceptance:
  - Clean machine can build and run with README Quick Start
  - Default config loads without errors

2) Policy validation and safe defaults
- Status: Done
- Scope: clear validation errors for malformed policy files
- Acceptance:
  - Invalid policy fails fast with actionable error output

3) Ledger integrity baseline
- Status: Done
- Scope: strict sequence checks, hash chaining, signing, verify
- Acceptance:
  - `logyctl verify` passes after a normal run

4) Admin endpoint protection
- Status: Done
- Scope: token protection for rekey endpoint
- Acceptance:
  - Unauthorized requests return 401

5) Graceful shutdown
- Status: Done
- Scope: drain queues, flush ledger, clean stop
- Acceptance:
  - No data loss on SIGINT/SIGTERM with active load

---

## Reliability and observability

6) Metrics expansion
- Status: Done
- Scope: queue depth, drop rate, latency histograms
- Acceptance:
  - Prometheus exposes latency and queue depth metrics

7) Structured logging
- Status: Done
- Scope: correlation IDs per request/task, log levels
- Acceptance:
  - Logs include request_id and task_id when available

8) Health and readiness probes
- Status: Done
- Scope: `/healthz` and `/readyz`
- Acceptance:
  - Readiness fails if signer or DB is unavailable

9) Backpressure strategy
- Status: Done
- Scope: configurable drop vs block policy, metrics
- Acceptance:
  - Backpressure strategy is configurable and observable
  - Prometheus exposes backpressure mode and blocked submit counters

---

## Security and trust

10) Key management workflow
- Status: Done
- Scope: rotate, backup, restore, document handling
- Acceptance:
  - Rotation does not break chain verification
  - Restore workflow is documented and tested
  - CLI commands: backup-key, restore-key, list-backups
  - KEY_MANAGEMENT.md guide with security best practices

11) Evidence bag signing
- Status: Planned
- Scope: signed manifest for export
- Acceptance:
  - Evidence bag signature can be verified independently

12) Threat model and risk register
- Status: Planned
- Scope: assumptions, adversary model, mitigations
- Acceptance:
  - Threat model document reviewed and included

13) Audit trail access control
- Status: Backlog
- Scope: local user/role access for CLI and exports
- Acceptance:
  - Audit access is restricted by role or token

---

## Ledger scalability and performance

14) Batch writes and flushing
- Status: Planned
- Scope: buffered inserts with bounded latency
- Acceptance:
  - Sustained throughput improves with same integrity guarantees

15) Retention policies
- Status: Planned
- Scope: time/size based retention with audit records
- Acceptance:
  - Retention deletes include audit metadata

16) Storage pluggability
- Status: Backlog
- Scope: PostgreSQL backend via EventRepository
- Acceptance:
  - Storage tests pass for SQLite and PostgreSQL

17) Indexing and query optimization
- Status: Backlog
- Scope: indexes for risk, task, time, method
- Acceptance:
  - Trace and risk queries remain fast under load

---

## Policy engine evolution

18) Advanced conditions
- Status: Planned
- Scope: AND/OR groups, numeric ranges, string match modes
- Acceptance:
  - Policy test suite covers edge cases and nested logic

19) Redaction rules v2
- Status: Planned
- Scope: regex, partial masking, structured key paths
- Acceptance:
  - Redaction is deterministic and verifiable

20) Policy test harness
- Status: Planned
- Scope: sample policies + fixtures
- Acceptance:
  - CI validates policy behavior with fixtures

---

## Investigator and analyst UX

21) CLI filtering
- Status: Planned
- Scope: filter by risk, actor, method, time
- Acceptance:
  - Filters produce expected subsets across sample data

22) HTML report improvements
- Status: Planned
- Scope: summary, timeline, policy references, chain hash
- Acceptance:
  - Report includes chain hash and policy IDs

23) Replay safety tools
- Status: Backlog
- Scope: dry‑run mode, diff output, deterministic replays
- Acceptance:
  - Replay can run without side effects and show diffs

---

## Platform and release engineering

24) Release artifacts and checksums
- Status: Done
- Scope: signed checksums for releases, SBOM generation
- Acceptance:
  - Release includes checksums and verification steps
  - GoReleaser generates SHA256 checksums and SPDX SBOM

25) CI quality gates
- Status: Done
- Scope: lint, static analysis, tests, coverage, race detector
- Acceptance:
  - CI fails on new warnings or lint errors
  - 5 parallel jobs: tests, safety checks, lint, build validation, integration

26) Security scanning
- Status: Backlog
- Scope: dependency and SAST scanning
- Acceptance:
  - No critical findings in CI reports

---

## Ecosystem readiness

27) Integration guides
- Status: Planned
- Scope: agent integration examples
- Acceptance:
  - At least two working examples documented

28) Compatibility matrix
- Status: Backlog
- Scope: supported OS and Go versions
- Acceptance:
  - CI validates the matrix and documents it

29) Support and handoff kit
- Status: Planned
- Scope: known limitations, troubleshooting, contacts
- Acceptance:
  - Handoff doc covers setup, common issues, and escalation paths

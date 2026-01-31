# Key Management Guide

This document describes how to manage Ed25519 signing keys used for cryptographic integrity of the Logryph ledger.

## Overview

Logryph uses Ed25519 digital signatures to ensure tamper-evident ledger integrity. Each event is signed with a private key stored in `.logryph_key`. The corresponding public key is embedded in the ledger and used for verification.

## Key Lifecycle

### Initial Key Generation

When Logryph starts for the first time, it automatically generates a new Ed25519 keypair:
- Private key: Saved to `.logryph_key` (hex-encoded, 0600 permissions)
- Public key: Embedded in genesis block and run metadata

No manual intervention required.

### Key Rotation

Rotate keys periodically or immediately after suspected compromise:

```bash
# 1. Backup current key FIRST
logryph-cli backup-key

# 2. Rotate to new keypair (requires Logryph running)
logryph-cli rekey

# Output shows old and new public keys:
# {"old_public_key":"abc123...","new_public_key":"def456..."}
```

**Important**: Key rotation creates a discontinuity in the signature chain. Events signed with the old key remain valid, but you must retain the old public key for verification.

### Backup Workflow

Create timestamped backups before rotation or as part of regular maintenance:

```bash
# Create backup (saves to .logryph_key.backup.<timestamp>)
logryph-cli backup-key

# Output:
# Key backed up to: .logryph_key.backup.20260127T143022Z
# Store this backup securely (offline storage recommended)
```

**Best practices**:
- Backup before every key rotation
- Store backups offline (USB drive, hardware security module, air-gapped system)
- Encrypt backups at rest (GPG, age, or full-disk encryption)
- Maintain backup retention policy (e.g., keep last 3 backups)

### Restore Workflow

Restore from backup after key loss or corruption:

```bash
# 1. Stop Logryph (REQUIRED - prevents key mismatch during signing)
pkill logryph

# 2. List available backups
logryph-cli list-backups

# Output:
# Key Backups
# ===========
# .logryph_key.backup.20260127T143022Z (64 bytes, 2026-01-27 14:30:22)
# .logryph_key.backup.20260120T091500Z (64 bytes, 2026-01-20 09:15:00)

# 3. Restore from specific backup
logryph-cli restore-key .logryph_key.backup.20260127T143022Z

# Output:
# Existing key moved to: .logryph_key.old
# Key restored successfully
# Warning: Chain verification will fail for events signed with the old key

# 4. Restart Logryph
./logryph
```

**Warning**: Restoring an old key after new events have been signed will break chain verification. Only restore if:
- You lost the current key and need to resume operations
- You are rolling back to a known-good state
- You understand the verification implications

## Verification After Rotation

After key rotation, verify ledger integrity:

```bash
# Full chain verification (checks all signatures)
logryph-cli verify

# If rotation was recent, verification will show:
# - Events 0-N: Signed with old key (valid)
# - Events N+1-M: Signed with new key (valid)
# Overall: PASS (both keys valid for their respective ranges)
```

Logryph CLI handles multi-key verification automatically by extracting public keys from run metadata.

## Security Considerations

### Key Storage

- **DO**: Store `.logryph_key` with 0600 permissions (owner read/write only)
- **DO**: Use full-disk encryption on systems storing keys
- **DO NOT**: Commit `.logryph_key` to version control (already in .gitignore)
- **DO NOT**: Share private keys via email, Slack, or unencrypted channels

### Key Rotation Triggers

Rotate keys immediately if:
- Private key exposure suspected (e.g., compromised system, leaked backup)
- Employee with key access leaves organization
- Regulatory compliance requires periodic rotation (e.g., annually)

Rotate keys proactively:
- Every 90 days (standard practice)
- Before major releases or audits
- After significant security incidents (even if unrelated)

### Backup Security

- Encrypt backups: `gpg -c .logryph_key.backup.<timestamp>`
- Store offline: Air-gapped USB drives, hardware wallets, paper printouts
- Geographically distribute: Keep backups in multiple physical locations
- Test restores: Verify backup integrity quarterly

## Multi-Signature Setup (Future)

Logryph currently uses single-key signing. Future versions may support:
- Threshold signatures (m-of-n keys required)
- Hardware security modules (HSM) integration
- Key splitting (Shamir secret sharing)

## Troubleshooting

### "Failed to load key" error on startup

Likely cause: `.logryph_key` deleted or corrupted.

Solution:
1. Restore from backup: `logryph-cli restore-key <backup-file>`
2. If no backup exists, Logryph will generate new key (WARNING: breaks chain verification for old events)

### "Signature verification failed" after restore

Likely cause: Restored key does not match the key that signed recent events.

Solution:
1. Check you restored the correct backup (most recent)
2. If intentional rollback, accept that newer events will fail verification
3. If unintentional, restore the newer key

### Key rotation during high load

Recommendation: Rotate during maintenance windows. While rotation does not block event processing, it introduces a small risk of signature inconsistency if events are in-flight during the key swap.

## Compliance and Auditing

### Audit Trail

Key operations are logged to Logryph structured logs:
```json
{"level":"info","component":"api","event":"rekey_success","old_pubkey":"abc...","new_pubkey":"def...","timestamp":"2026-01-27T14:30:22Z"}
```

Filter key management events:
```bash
cat logryph.log | grep rekey
```

### Evidence Bag Exports

When exporting evidence bags, include:
- Current public key (embedded in metadata.json)
- Genesis block public key (for chain verification)
- Key rotation events (if any occurred during the run)

External auditors can verify signatures using only the public key (private key never shared).

## References

- Ed25519 Specification: RFC 8032
- Key Management Best Practices: NIST SP 800-57
- Logryph Architecture: ARCHITECTURE.md

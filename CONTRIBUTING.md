# Contributing to Logryph

We are building the safety layer for the Agentic Era. We welcome contributions from everyone, especially those with experience in security, cryptography, and distributed systems.

## Getting Started

1.  **Fork the repo** and clone it locally.
2.  Install **Go 1.22+**.
3.  Run `go mod download` to install dependencies.

## Development Workflow

### 1. Run the Safety Checks
Before writing code, make sure your environment passes our compliance checks:
```bash
./scripts/safety-check.sh
```

### 2. Make Changes
*   Keep functions under 60 lines (NASA Rule #4).
*   Use `assert.Check` heavily (Goal: 2.0 assertion density).
*   Avoid recursion and unbounded loops.

### 3. Run Tests
```bash
go test -v ./...
```

## Pull Request Process

1.  Create a branch: `git checkout -b feature/amazing-feature`.
2.  Commit your changes (please use conventional commits, e.g., `feat: add new policy`).
3.  Push to the branch: `git push origin feature/amazing-feature`.
4.  Open a Pull Request.

## Safety First
Logryph is **safety-critical software**. All PRs must maintain or improve the safety score. Features that introduce memory leaks, race conditions, or unchecked errors will be rejected.

## Release Process (Maintainers Only)

### Automated Releases

Releases are automated via GitHub Actions when a version tag is pushed:

```bash
# 1. Ensure main branch is clean and all CI checks pass
git checkout main
git pull origin main

# 2. Create and push a version tag (semantic versioning)
git tag v0.1.0
git push origin v0.1.0
```

This triggers:
- Multi-platform builds (Linux, macOS, Windows for amd64/arm64)
- SBOM (Software Bill of Materials) generation
- SHA256 checksum generation
- Automatic GitHub release with changelog
- Archive creation with LICENSE, README, and sample configs

### Pre-Release Checklist

- [ ] All CI checks passing on main
- [ ] Safety checks clean (`./scripts/safety-check.sh`)
- [ ] Version number follows semantic versioning
- [ ] CHANGELOG.md updated (if maintained manually)
- [ ] No open security issues

### Testing a Release Locally

```bash
# Test the build without publishing
goreleaser build --snapshot --clean --single-target

# Check the dist/ folder for artifacts
ls -lah dist/
```

### Manual Release (Emergency Only)

If automated release fails, trigger manually:
1. Go to GitHub Actions → Release workflow
2. Click "Run workflow"
3. Ensure the tag exists first

### Release Artifacts

Each release includes:
- `logryph_<version>_<os>_<arch>.tar.gz` - Main proxy binary
- `logryph-cli_<version>_<os>_<arch>.tar.gz` - CLI tool binary
- `checksums.txt` - SHA256 checksums for verification
- `logryph-sbom.spdx.json` - Software Bill of Materials (supply chain security)

### Verifying a Release

Users can verify release integrity:
```bash
# Download release and checksums
wget https://github.com/[org]/logryph/releases/download/v0.1.0/logryph_0.1.0_linux_x86_64.tar.gz
wget https://github.com/[org]/logryph/releases/download/v0.1.0/checksums.txt

# Verify checksum
sha256sum -c checksums.txt --ignore-missing
```

## License
By contributing, you agree that your contributions will be licensed under its Apache 2.0 License.

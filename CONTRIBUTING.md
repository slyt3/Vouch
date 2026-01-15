# Contributing to Vouch

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
Vouch is **safety-critical software**. All PRs must maintain or improve the safety score. Features that introduce memory leaks, race conditions, or unchecked errors will be rejected.

## License
By contributing, you agree that your contributions will be licensed under its Apache 2.0 License.

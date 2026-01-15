# Vouch: Cloud Deployment & Production Ops

Vouch is designed to be cloud-native, lightweight, and extremely easy to deploy in production environments.

## Containerization (Docker)

The recommended way to run Vouch in the cloud is via Docker.

```dockerfile
# Build Stage
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY . .
RUN go build -o vouch-proxy .

# Production Stage
FROM alpine:latest
RUN apk add --no-cache ca-certificates
WORKDIR /root/
COPY --from=builder /app/vouch-proxy .
COPY --from=builder /app/vouch-policy.yaml .
COPY --from=builder /app/schema.sql .

EXPOSE 9999 9998
CMD ["./vouch-proxy"]
```

## CI/CD Pipeline

Vouch uses GitHub Actions for automated quality assurance.

*   **Workflow**: `.github/workflows/ci.yml`
*   **Triggers**: Every push and PR.
*   **Checks**: `go test`, `go vet`, and cross-platform build verification.

## Production Operations

### 1. Persistence & Volumes
Vouch stores its ledger in `vouch.db`. In a cloud environment (Kubernetes/ECS), this **must** be mounted to a persistent volume (EBS/PVC) to ensure logs are not lost on container restart.

### 2. Monitoring
Vouch emits structured logs to `stdout`. 
*   **Backpressure Metrics**: Monitor logs for `[BACKPRESSURE]`. If this appears, increase the persistence layer's IOPS.
*   **Health Checks**: Use the `/api/health` (proposed) or monitor for `[CRITICAL]` ledger failure logs.

### 3. Key Management
The `.vouch_key` is the root of trust.
*   **Security**: Do not commit this key to version control.
*   **Rotation**: Use the `vouch-cli rekey` command periodically to rotate the Ed25519 pair.

## Architecture Patterns

| Pattern | Implementation |
| :--- | :--- |
| **Sidecar** | Run Vouch as a sidecar container to your AI Agent service. |
| **Gateway** | Run Vouch as a centralized gateway for multiple agents. |
| **Offline** | Run Vouch locally for development and periodic audit uploads. |

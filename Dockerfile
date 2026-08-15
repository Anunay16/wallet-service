# ─── Stage 1: Build ───────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache dependency downloads separately from source code
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

# Produce a statically linked binary (no CGO, stripped debug symbols)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o wallet-server ./cmd/server

# ─── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.20

# Create non-root user and group
RUN addgroup -S wallet && adduser -S wallet -G wallet

WORKDIR /app

COPY --from=builder /app/wallet-server .
COPY --from=builder /app/config.yaml   .
COPY --from=builder /app/ui/          ./ui/

RUN chown -R wallet:wallet /app

# Switch to non-root
USER wallet

EXPOSE 8080

# wget is available in alpine — hits the /health endpoint
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/wallet-server"]

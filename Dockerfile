# ── build stage ──────────────────────────────────────────────────────
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /server ./cmd/server

# ── runtime stage ────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates wget
WORKDIR /app
COPY --from=build /server /app/server
COPY migrations/ /app/migrations/

# Web build is copied in by CI (actions/download-artifact) before
# docker build runs. If deploy/web is empty the server still works —
# it just won't serve the static client (set STATIC_DIR="" to skip).
COPY deploy/web/ /app/web/

EXPOSE 8080
ENV LISTEN_ADDR=:8080 \
    STATIC_DIR=/app/web \
    AUTH_SECRET="" \
    LEDGER_DSN="" \
    TRUST_PROXY="true"

HEALTHCHECK --interval=15s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/v1/healthz || exit 1

ENTRYPOINT ["/app/server"]

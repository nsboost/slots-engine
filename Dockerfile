# ---------- build stage ----------
FROM golang:1.22-alpine AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

# ---------- runtime stage ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /server /app/server
COPY migrations/ /app/migrations/

# Only copy static assets if they were generated.
RUN mkdir -p /app/web
COPY . .
RUN if [ -d /app/deploy/web ]; then cp -a /app/deploy/web/. /app/web/; fi

EXPOSE 8080
ENV LISTEN_ADDR=:8080 \
    STATIC_DIR=/app/web \
    AUTH_SECRET="" \
    LEDGER_DSN="" \
    TRUST_PROXY="true"

# Copy web build if it exists at build time (CI copies it in before docker build).
# When running without a web build, set STATIC_DIR to an empty string.
COPY deploy/web/ /app/web/

EXPOSE 8080
ENV LISTEN_ADDR=:8080 \
    STATIC_DIR=/app/web \
    AUTH_SECRET="" \
    LEDGER_DSN="" \
    TRUST_PROXY="true"

HEALTHCHECK --interval=15s --timeout=5s CMD wget -qO- http://localhost:8080/v1/healthz || exit 1

ENTRYPOINT ["/app/server"]

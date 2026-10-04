# slots-engine

Server-authoritative slot machine backend: game math, double-entry coin
ledger, and an HTTP API for a mobile client. Built as a **social casino**
(play-money, no cash value) demo first; the architecture is laid out so
that real-money cash-out is a licensing + config change, not a rewrite.

**Proprietary — All Rights Reserved.** See [LICENSE](./LICENSE). This is a
private repository; do not share code or binaries outside agreed
arrangements.

## Status

| Phase | What | Status |
|---|---|---|
| 1 | Slot math engine (reels, paytable, RTP) | ✅ Done — ~95.6-95.8% RTP verified at 10M simulated spins |
| 2 | Double-entry ledger + HTTP API | ✅ Done — in-memory store for dev, Postgres store for durable deploys |
| 3 | Mobile client (Godot) | Not started |
| 4 | Real-money cash-out (requires gaming license) | Not started — **do not enable without legal sign-off** |

## Why this structure

- `internal/engine` — the game math. Server-authoritative: a client never
  computes outcomes, only renders what the server returns. This is the
  one package a certification lab will eventually test directly, so it
  has no dependency on the ledger, the API, or any storage.
- `internal/ledger` — a double-entry accounting system for coin balances.
  Every coin movement is a balanced `Transaction` of `Entry` legs that sum
  to zero. Storage is behind the `Store` interface so the backend
  (in-memory now, Postgres for anything durable) can change without
  touching callers.
- `internal/api` — thin HTTP handlers. No game or money logic lives here
  — handlers validate input, call into `engine`/`ledger`, and serialize
  results. This keeps the core logic testable without spinning up HTTP.
- `cmd/server` — the API entrypoint. Chooses the ledger backend from
  `LEDGER_DSN` so swapping dev↔prod storage is a config change.
- `cmd/simulate` — Monte Carlo RTP/volatility simulator. **Run this after
  any reel strip or paytable change, before shipping it anywhere.**
- `cmd/play` — prints individual spins as plain text, for quick manual
  sanity checks.
- `migrations/` — SQL schema for the Postgres ledger backend.

Both `internal/engine` and `internal/ledger` are under `internal/`, which
the Go compiler enforces cannot be imported by any module outside this
one — relevant once this is a private repo with code we don't want
leaking into some other project by accident.

## Requirements

- Go 1.22+
- Postgres 14+ (only if running with `LEDGER_DSN` set; otherwise the
  in-memory store needs nothing)

## Running locally

```bash
# Build and run the API server (in-memory ledger, resets on restart)
go run ./cmd/server

# Or with a durable Postgres backend:
psql "$LEDGER_DSN" -f migrations/0001_init.sql   # once, to set up the schema
LEDGER_DSN="postgres://user:pass@localhost:5432/slots?sslmode=disable" go run ./cmd/server
```

Smoke test against a running server:

```bash
curl -s localhost:8080/v1/healthz

curl -s -X POST localhost:8080/v1/players/alice/demo-purchase -d '{"amount": 1000}'

curl -s -X POST localhost:8080/v1/players/alice/spin \
  -d '{"betPerLine":1,"linesPlayed":9,"idempotencyKey":"spin-1"}'

curl -s localhost:8080/v1/players/alice/balance
```

## RTP simulation

```bash
go run ./cmd/simulate -spins=5000000
```

Re-run this and check the reported RTP lands in the target band
(94-96% for this demo) after any change to `internal/engine/demo_game.go`.

## Tests

```bash
go vet ./...
go test ./...
```

`internal/ledger`'s tests run against the in-memory store. The Postgres
store (`PGStore`) implements the identical `Store` interface and should
get the same test scenarios run against a real database before it's
trusted in production — see the comment at the top of
`internal/ledger/ledger_test.go`.

## Known gaps before this is production-ready

These are tracked here rather than left implicit, so nothing gets
forgotten when picking this back up later:

- **Auth is not implemented.** `internal/api` assumes an authenticated
  player; there's no session/token verification yet.
- **Spin-result replay is partial.** Ledger writes are idempotent (a
  retried request never double-charges), but the spin *grid* returned on
  a replay is freshly generated, not the original. Fix: persist the
  `SpinResult` alongside its ledger transaction in the same DB write.
- **No payment processor integration.** The `demo-purchase` endpoint
  mints coins with no real money involved and must not exist in any
  build with real-money cash-out enabled.
- **No KYC/age/geofencing.** Required before any real-money mode.
- **No rate limiting / abuse protection** on the API.
- **PGStore has no automated tests yet** — needs a real Postgres instance
  (or test container) wired into CI.

See `/areas/slots-game.md` in project notes for the full phase plan.

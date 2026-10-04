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
| 2 | Double-entry ledger + HTTP API | ✅ Done — in-memory store for dev, Postgres store (integration-tested) for durable deploys; dev-grade auth and per-player rate limiting in place |
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

- **Auth is dev-grade.** `StaticKeyAuthenticator` (a single static key per
  player, sent as `X-Player-Key`) stands in for real session/token
  verification. Replace with Apple/Google sign-in-issued, server-verified
  session tokens before anything beyond closed demo testing. The
  `Authenticator` interface and `RequireAuth` middleware are already
  structured so this is a one-file swap, not a rewrite — and the
  cross-player protection (a valid key for player A can't touch player
  B's path) is already enforced regardless of which `Authenticator` is
  plugged in.
- **Rate limiting is per-process, in-memory.** Fine for a single server
  instance; replace with a shared store (Redis `INCR`+`EXPIRE` is the
  standard pattern) before running more than one instance behind a load
  balancer, or each instance will enforce the limit independently.
- **No payment processor integration.** The `demo-purchase` endpoint
  mints coins with no real money involved and must not exist in any
  build with real-money cash-out enabled.
- **No KYC/age/geofencing.** Required before any real-money mode.

## Closed this session

- **Spin-result replay is now fully idempotent.** The spin outcome
  (`SpinResult`) is persisted as the ledger transaction's `Payload`, and
  the API checks for an existing transaction *before* calling the engine.
  A retried request now returns the exact original grid and wins, not a
  freshly rolled outcome tied to an already-settled bet. Covered by
  `TestSpinReplayReturnsIdenticalGrid` and
  `TestSpinReplayDoesNotDoubleCharge` in `internal/api/server_test.go`.
- **Auth and cross-player protection implemented** (`internal/api/auth.go`).
  Every player-scoped route requires a valid `X-Player-Key` and rejects a
  valid key used against a different player's path with 403 (distinct
  from 401 for a missing/invalid key — this distinction matters for
  monitoring: 403s are a potential attack signal, 401s usually aren't).
- **Rate limiting implemented** (`internal/api/ratelimit.go`), keyed by
  authenticated player ID, 60 req/min by default.
- **PGStore now has a real integration test suite**, run against an
  actual Postgres instance (`internal/ledger/pgstore_integration_test.go`,
  build-tagged `integration` so it doesn't run without a database
  configured). Covers mint/spend, idempotent replay with payload,
  insufficient-funds rejection, and — importantly — 10 concurrent spins
  against one balance to confirm the `SELECT ... FOR UPDATE` row locking
  actually prevents lost updates. All four passed, including under `-race`.

  Run it yourself with a scratch database:
  ```bash
  psql "$PGTEST_DSN" -f migrations/0001_init.sql
  PGTEST_DSN="postgres://user:pass@localhost:5432/dbname?sslmode=disable" \
    go test -tags=integration ./internal/ledger/... -run Postgres -v -race
  ```

See `/areas/slots-game.md` in project notes for the full phase plan.

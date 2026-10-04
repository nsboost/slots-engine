# Architecture

## Layering

```
cmd/server  ──┐
cmd/simulate ─┼──> internal/engine   (pure math, no I/O, no deps)
cmd/play    ──┘          │
                          │ (SpinResult)
              internal/api ──> internal/ledger ──> Store interface
                                                      ├── MemStore (dev/test)
                                                      └── PGStore (prod)
```

Dependencies only point downward. `internal/engine` knows nothing about
money or HTTP. `internal/ledger` knows nothing about game math or HTTP.
`internal/api` is the only layer that knows about both, and it contains
no business logic of its own — it just wires requests to the two domain
packages and serializes results.

This matters for three concrete reasons:

1. **Certification.** A slot math certification lab (GLI, BMM, etc.)
   tests `internal/engine` in isolation. It shouldn't need a database or
   an HTTP server to do that, and it doesn't.
2. **Portability.** If the backend language ever changes, the engine and
   ledger are the two pieces with real intellectual value to port
   carefully; the HTTP layer is thin enough to rewrite in an afternoon.
3. **Testability.** Every test in this repo runs without starting an HTTP
   server, and the ledger tests run without a database.

## The ledger is double-entry on purpose

Every balance change is a `Transaction` of `Entry` legs whose `Amount`
fields sum to exactly zero. This is the standard real-money accounting
model, adopted now — while the game is play-money only — so that turning
on real cash-out later is a matter of:

- adding a payment-provider webhook that calls `MintTransaction` instead
  of `demo-purchase`'s client-triggered one,
- adding a withdrawal flow that debits `PLAYER_PURCHASED` and credits an
  external payout record,
- and getting a gaming license —

not a rewrite of how balances work.

`SYSTEM_MINT` is the one account intentionally allowed to go negative
without bound: it's the counterparty for every coin ever created, so its
balance is always `-1 × (total coins ever minted)`. If `SYSTEM_MINT`'s
balance ever stops being exactly the negative of the sum of all player
and house balances, something has bypassed the ledger package and
written to storage directly — that invariant is the reconciliation check
referenced in `migrations/0001_init.sql`.

## Idempotency

Every ledger-affecting API call takes a caller-supplied idempotency key
(`Transaction.ID`). `Store.Apply` is required to detect a repeat and
return the original result rather than applying twice — mobile clients
on flaky connections *will* retry.

Current limitation (tracked in the README): the ledger write is fully
idempotent, but the *spin outcome* (which symbols landed) is not yet
persisted alongside it, so a replayed request gets a fresh random grid
tied to the same (already-applied) financial transaction. Fix before
production: write `SpinResult` and the `ledger.Transaction` in the same
database transaction, keyed by the same idempotency key, and have a
replay fetch the stored result instead of calling `Engine.Spin` again.

## Why no web framework, no ORM

- Go 1.22's `net/http.ServeMux` supports method + path-pattern routing
  natively (`"POST /v1/players/{playerID}/spin"`), which covers this
  API's needs without adding a router dependency to audit and keep
  patched.
- `database/sql` + `lib/pq` is plain SQL, not an ORM. For a ledger, the
  exact SQL executed (especially locking behavior — see `PGStore.Apply`'s
  `SELECT ... FOR UPDATE`) matters enough that hiding it behind an ORM's
  query builder would cost more than it saves.

Fewer dependencies also means less to re-vet every time this moves to a
new environment, and less surface area for a supply-chain issue in code
that moves real money.

## RTP tuning workflow

1. Change `internal/engine/demo_game.go` (reel strip weights or
   `Paytable`/`ScatterPaytable` values).
2. `go run ./cmd/simulate -spins=5000000` (or higher for a tighter
   confidence interval near the target band's edges).
3. Confirm RTP and hit frequency look right for the target audience.
4. `go test ./...` — `TestRTPRoughlyInBand` is a fast, loose guard that
   runs in CI; it will NOT catch a subtle miscalibration, only a gross
   regression. Always use `cmd/simulate` for real tuning decisions.
5. Bump `engine.GameVersion`.

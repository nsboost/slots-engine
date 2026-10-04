package ledger

import "context"

// Store is the persistence boundary for the ledger. Swapping backends
// (in-memory -> Postgres, or Postgres -> something else later) means
// writing one new implementation of this interface — nothing above this
// layer (the API, the engine) ever needs to change.
type Store interface {
	// Apply validates and applies a transaction atomically. Applying the
	// same Transaction.ID twice must be a no-op that returns the original
	// result (idempotency) rather than double-applying — this matters
	// because clients WILL retry requests on timeout.
	Apply(ctx context.Context, tx Transaction) (ApplyResult, error)

	// Balance returns the current balance of one account. Returns 0 for an
	// account that has never been touched (not an error).
	Balance(ctx context.Context, acc Account) (int64, error)

	// Balances returns balances for every account kind belonging to one
	// player in a single call, to avoid N round trips.
	Balances(ctx context.Context, playerID string) (map[AccountKind]int64, error)

	// History returns the most recent transactions touching a player's
	// accounts, newest first, for support/audit lookups.
	History(ctx context.Context, playerID string, limit int) ([]Transaction, error)
}

// ApplyResult reports what Apply did, so callers can distinguish a fresh
// apply from a replayed idempotent one.
type ApplyResult struct {
	Transaction Transaction
	Replayed    bool // true if this Transaction.ID had already been applied
}

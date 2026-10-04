package ledger

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/lib/pq" // Postgres driver, registered via database/sql
)

// PGStore is a Postgres-backed Store. See migrations/0001_init.sql for the
// schema. Durable and the intended backend for anything beyond local dev.
//
// Transaction isolation: Apply runs in a single SQL transaction with a
// row-level lock (SELECT ... FOR UPDATE) on every touched balance row, so
// concurrent spins from the same player serialize correctly instead of
// racing on a read-modify-write.
type PGStore struct {
	db *sql.DB
}

// OpenPGStore opens a connection pool against dsn (a standard Postgres
// connection string) and verifies connectivity. It does NOT run
// migrations — run migrations/0001_init.sql separately (e.g. via a
// migration tool or `psql -f`) as part of deploy, not at process start,
// so schema changes stay reviewable and don't race multiple instances
// migrating at once.
func OpenPGStore(dsn string) (*PGStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("ledger: open postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ledger: ping postgres: %w", err)
	}
	return &PGStore{db: db}, nil
}

func (s *PGStore) Close() error { return s.db.Close() }

func (s *PGStore) Apply(ctx context.Context, tx Transaction) (ApplyResult, error) {
	if err := tx.Validate(); err != nil {
		return ApplyResult{}, err
	}

	dbtx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ApplyResult{}, fmt.Errorf("ledger: begin tx: %w", err)
	}
	defer dbtx.Rollback() //nolint:errcheck // no-op if already committed

	// Idempotency check.
	var existingType string
	err = dbtx.QueryRowContext(ctx, `SELECT type FROM transactions WHERE id = $1`, tx.ID).Scan(&existingType)
	if err == nil {
		// Already applied — fetch and return the full prior transaction.
		prior, ferr := s.fetchTransaction(ctx, dbtx, tx.ID)
		if ferr != nil {
			return ApplyResult{}, ferr
		}
		return ApplyResult{Transaction: prior, Replayed: true}, nil
	} else if err != sql.ErrNoRows {
		return ApplyResult{}, fmt.Errorf("ledger: idempotency check: %w", err)
	}

	if _, err := dbtx.ExecContext(ctx,
		`INSERT INTO transactions (id, type, ref) VALUES ($1, $2, $3)`,
		tx.ID, tx.Type, tx.Ref,
	); err != nil {
		return ApplyResult{}, fmt.Errorf("ledger: insert transaction: %w", err)
	}

	// Net each account's delta within this transaction first (an account
	// can appear in multiple entries), then lock and update each touched
	// account exactly once, in a stable (sorted) order to avoid deadlocks
	// between concurrent transactions touching overlapping accounts.
	deltas := make(map[string]int64)
	accountOf := make(map[string]Account)
	for _, e := range tx.Entries {
		k := e.Account.Key()
		deltas[k] += e.Amount
		accountOf[k] = e.Account

		if _, err := dbtx.ExecContext(ctx,
			`INSERT INTO entries (transaction_id, player_id, account_kind, amount) VALUES ($1, $2, $3, $4)`,
			tx.ID, e.Account.PlayerID, string(e.Account.Kind), e.Amount,
		); err != nil {
			return ApplyResult{}, fmt.Errorf("ledger: insert entry: %w", err)
		}
	}

	keys := sortedKeys(deltas)
	for _, k := range keys {
		acc := accountOf[k]
		var current int64
		err := dbtx.QueryRowContext(ctx,
			`SELECT balance FROM balances WHERE player_id = $1 AND account_kind = $2 FOR UPDATE`,
			acc.PlayerID, string(acc.Kind),
		).Scan(&current)
		if err == sql.ErrNoRows {
			current = 0
			if _, err := dbtx.ExecContext(ctx,
				`INSERT INTO balances (player_id, account_kind, balance) VALUES ($1, $2, 0)`,
				acc.PlayerID, string(acc.Kind),
			); err != nil {
				return ApplyResult{}, fmt.Errorf("ledger: seed balance row: %w", err)
			}
		} else if err != nil {
			return ApplyResult{}, fmt.Errorf("ledger: lock balance: %w", err)
		}

		next := current + deltas[k]
		// Only player-owned accounts are forbidden from going negative;
		// house/system accounts (empty PlayerID) may legitimately go
		// negative — see the matching comment in MemStore.Apply.
		if acc.PlayerID != "" && next < 0 {
			return ApplyResult{}, ErrInsufficientFunds
		}
		if _, err := dbtx.ExecContext(ctx,
			`UPDATE balances SET balance = $1 WHERE player_id = $2 AND account_kind = $3`,
			next, acc.PlayerID, string(acc.Kind),
		); err != nil {
			return ApplyResult{}, fmt.Errorf("ledger: update balance: %w", err)
		}
	}

	if err := dbtx.Commit(); err != nil {
		return ApplyResult{}, fmt.Errorf("ledger: commit: %w", err)
	}

	return ApplyResult{Transaction: tx, Replayed: false}, nil
}

func (s *PGStore) Balance(ctx context.Context, acc Account) (int64, error) {
	var bal int64
	err := s.db.QueryRowContext(ctx,
		`SELECT balance FROM balances WHERE player_id = $1 AND account_kind = $2`,
		acc.PlayerID, string(acc.Kind),
	).Scan(&bal)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ledger: balance query: %w", err)
	}
	return bal, nil
}

func (s *PGStore) Balances(ctx context.Context, playerID string) (map[AccountKind]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_kind, balance FROM balances WHERE player_id = $1`, playerID,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: balances query: %w", err)
	}
	defer rows.Close()

	out := map[AccountKind]int64{KindPlayerPurchased: 0, KindPlayerBonus: 0}
	for rows.Next() {
		var kind string
		var bal int64
		if err := rows.Scan(&kind, &bal); err != nil {
			return nil, fmt.Errorf("ledger: scan balance row: %w", err)
		}
		out[AccountKind(kind)] = bal
	}
	return out, rows.Err()
}

func (s *PGStore) History(ctx context.Context, playerID string, limit int) ([]Transaction, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT t.id, t.type, t.ref, t.created_at
		 FROM transactions t JOIN entries e ON e.transaction_id = t.id
		 WHERE e.player_id = $1
		 ORDER BY t.created_at DESC
		 LIMIT $2`,
		playerID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: history query: %w", err)
	}
	defer rows.Close()

	var out []Transaction
	for rows.Next() {
		var tx Transaction
		if err := rows.Scan(&tx.ID, &tx.Type, &tx.Ref, &tx.CreatedAt); err != nil {
			return nil, fmt.Errorf("ledger: scan history row: %w", err)
		}
		out = append(out, tx)
	}
	return out, rows.Err()
}

func (s *PGStore) fetchTransaction(ctx context.Context, dbtx *sql.Tx, id string) (Transaction, error) {
	var tx Transaction
	err := dbtx.QueryRowContext(ctx,
		`SELECT id, type, ref, created_at FROM transactions WHERE id = $1`, id,
	).Scan(&tx.ID, &tx.Type, &tx.Ref, &tx.CreatedAt)
	if err != nil {
		return Transaction{}, fmt.Errorf("ledger: fetch transaction: %w", err)
	}

	rows, err := dbtx.QueryContext(ctx,
		`SELECT player_id, account_kind, amount FROM entries WHERE transaction_id = $1`, id,
	)
	if err != nil {
		return Transaction{}, fmt.Errorf("ledger: fetch entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var e Entry
		var kind string
		if err := rows.Scan(&e.Account.PlayerID, &kind, &e.Amount); err != nil {
			return Transaction{}, fmt.Errorf("ledger: scan entry: %w", err)
		}
		e.Account.Kind = AccountKind(kind)
		tx.Entries = append(tx.Entries, e)
	}
	return tx, rows.Err()
}

func sortedKeys(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Simple insertion sort — these maps are tiny (a handful of entries
	// per spin transaction), so avoiding a sort.Strings import here isn't
	// worth it; use sort.Strings for clarity.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}

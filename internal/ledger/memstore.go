package ledger

import (
	"context"
	"sync"
)

// MemStore is a thread-safe, in-process Store implementation. It is the
// default for local dev and for unit/integration tests — no database
// required. It is NOT durable: a process restart loses all state. Use
// PGStore for anything that needs to survive a restart.
type MemStore struct {
	mu       sync.Mutex
	balances map[string]int64       // Account.Key() -> balance
	applied  map[string]Transaction // Transaction.ID -> the transaction, for idempotency + history
	order    []string               // Transaction.ID in apply order, for History()
	byPlayer map[string][]string    // playerID -> Transaction.IDs touching them, newest last
}

func NewMemStore() *MemStore {
	return &MemStore{
		balances: make(map[string]int64),
		applied:  make(map[string]Transaction),
		byPlayer: make(map[string][]string),
	}
}

func (s *MemStore) Apply(ctx context.Context, tx Transaction) (ApplyResult, error) {
	if err := tx.Validate(); err != nil {
		return ApplyResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.applied[tx.ID]; ok {
		return ApplyResult{Transaction: existing, Replayed: true}, nil
	}

	// Pre-check: a PLAYER-owned account may never go negative. House and
	// system accounts (empty PlayerID) are intentionally allowed to go
	// negative — SYSTEM_MINT in particular is an infinite source that
	// balances every coin ever minted to a player, by design. Check
	// before mutating so a failed transaction never partially applies.
	deltas := make(map[string]int64)
	accountOf := make(map[string]Account)
	for _, e := range tx.Entries {
		deltas[e.Account.Key()] += e.Amount
		accountOf[e.Account.Key()] = e.Account
	}
	for key, delta := range deltas {
		acc := accountOf[key]
		if acc.PlayerID != "" && s.balances[key]+delta < 0 {
			return ApplyResult{}, ErrInsufficientFunds
		}
	}

	for key, delta := range deltas {
		s.balances[key] += delta
	}

	s.applied[tx.ID] = tx
	s.order = append(s.order, tx.ID)

	seen := make(map[string]bool)
	for _, e := range tx.Entries {
		if e.Account.PlayerID == "" || seen[e.Account.PlayerID] {
			continue
		}
		seen[e.Account.PlayerID] = true
		s.byPlayer[e.Account.PlayerID] = append(s.byPlayer[e.Account.PlayerID], tx.ID)
	}

	return ApplyResult{Transaction: tx, Replayed: false}, nil
}

func (s *MemStore) Balance(ctx context.Context, acc Account) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.balances[acc.Key()], nil
}

func (s *MemStore) Balances(ctx context.Context, playerID string) (map[AccountKind]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[AccountKind]int64{
		KindPlayerPurchased: s.balances[Account{PlayerID: playerID, Kind: KindPlayerPurchased}.Key()],
		KindPlayerBonus:     s.balances[Account{PlayerID: playerID, Kind: KindPlayerBonus}.Key()],
	}
	return out, nil
}

func (s *MemStore) History(ctx context.Context, playerID string, limit int) ([]Transaction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := s.byPlayer[playerID]
	out := make([]Transaction, 0, limit)
	for i := len(ids) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.applied[ids[i]])
	}
	return out, nil
}

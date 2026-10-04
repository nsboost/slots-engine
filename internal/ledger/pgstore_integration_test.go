//go:build integration

// Run with: go test -tags=integration ./internal/ledger/... -run Postgres
// Requires PGTEST_DSN pointing at a scratch Postgres database with
// migrations/0001_init.sql already applied. Not part of the default
// `go test ./...` run (and so not part of CI unless a Postgres service is
// wired in) because it needs a real database — see README "Known gaps".
//
// These tests exist to prove PGStore behaves identically to MemStore for
// the scenarios that matter most: idempotency, insufficient-funds
// rejection, and balanced-transaction accounting. If you change either
// store's Apply logic, run both test files and confirm they agree.
package ledger

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
)

func openTestPGStore(t *testing.T) *PGStore {
	t.Helper()
	dsn := os.Getenv("PGTEST_DSN")
	if dsn == "" {
		t.Skip("PGTEST_DSN not set — skipping Postgres integration tests")
	}
	store, err := OpenPGStore(dsn)
	if err != nil {
		t.Fatalf("failed to open PGStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// uniqueID avoids collisions across repeated test runs against the same
// persistent database (unlike MemStore, PGStore's data outlives the
// test process).
func uniqueID(prefix string) string {
	return prefix + "-" + randHex(8)
}

func TestPostgresMintAndSpend(t *testing.T) {
	ctx := context.Background()
	s := openTestPGStore(t)
	player := uniqueID("player")

	mint, err := MintTransaction(uniqueID("mint"), player, 1000, KindPlayerPurchased, "DEMO_PURCHASE")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, mint); err != nil {
		t.Fatal(err)
	}

	bal, err := s.Balance(ctx, Account{PlayerID: player, Kind: KindPlayerPurchased})
	if err != nil {
		t.Fatal(err)
	}
	if bal != 1000 {
		t.Fatalf("expected balance 1000, got %d", bal)
	}

	spin, err := SpinSettlement(uniqueID("spin"), player, 10, 0, bal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, spin); err != nil {
		t.Fatal(err)
	}

	bal, _ = s.Balance(ctx, Account{PlayerID: player, Kind: KindPlayerPurchased})
	if bal != 990 {
		t.Fatalf("expected balance 990 after losing spin, got %d", bal)
	}
}

func TestPostgresIdempotentApplyWithPayload(t *testing.T) {
	ctx := context.Background()
	s := openTestPGStore(t)
	player := uniqueID("player")
	txID := uniqueID("mint")

	mint, _ := MintTransaction(txID, player, 500, KindPlayerPurchased, "DEMO_PURCHASE")
	mint.Payload = `{"example":"payload"}`

	r1, err := s.Apply(ctx, mint)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Replayed {
		t.Fatal("first apply should not be marked replayed")
	}

	r2, err := s.Apply(ctx, mint)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Replayed {
		t.Fatal("second apply with same ID should be replayed")
	}
	if r2.Transaction.Payload != `{"example":"payload"}` {
		t.Fatalf("replayed transaction lost its payload: %q", r2.Transaction.Payload)
	}

	bal, _ := s.Balance(ctx, Account{PlayerID: player, Kind: KindPlayerPurchased})
	if bal != 500 {
		t.Fatalf("balance must not double-apply: expected 500, got %d", bal)
	}

	// Get() must agree with what Apply's replay returned.
	fetched, ok, err := s.Get(ctx, txID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected Get to find the applied transaction")
	}
	if fetched.Payload != `{"example":"payload"}` {
		t.Fatalf("Get returned wrong payload: %q", fetched.Payload)
	}
}

func TestPostgresInsufficientFunds(t *testing.T) {
	ctx := context.Background()
	s := openTestPGStore(t)
	player := uniqueID("player")

	spin, err := SpinSettlement(uniqueID("spin"), player, 10, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, spin); err != ErrInsufficientFunds {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestPostgresConcurrentSpinsSerializeCorrectly(t *testing.T) {
	ctx := context.Background()
	s := openTestPGStore(t)
	player := uniqueID("player")

	mint, _ := MintTransaction(uniqueID("mint"), player, 100, KindPlayerPurchased, "DEMO_PURCHASE")
	if _, err := s.Apply(ctx, mint); err != nil {
		t.Fatal(err)
	}

	// Fire 10 concurrent 9-coin spins against a 100-coin balance. Exactly
	// 10 should succeed (100/9 = 11.1, so the 11th would fail, but we
	// only send 10 to keep this deterministic) and the final balance must
	// reflect all of them with no lost updates — this is what the
	// row-level locking in PGStore.Apply (SELECT ... FOR UPDATE) exists
	// to guarantee.
	const n = 10
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			spin, err := SpinSettlement(uniqueID("cspin"), player, 9, 0, 1_000_000) // purchasedBalance arg is just for the purchased/bonus split; actual check happens in Apply
			if err != nil {
				errs <- err
				return
			}
			_, err = s.Apply(ctx, spin)
			errs <- err
		}(i)
	}
	successes := 0
	for i := 0; i < n; i++ {
		if err := <-errs; err == nil {
			successes++
		}
	}

	bal, _ := s.Balance(ctx, Account{PlayerID: player, Kind: KindPlayerPurchased})
	expected := int64(100 - 9*successes)
	if bal != expected {
		t.Fatalf("lost update detected: expected balance %d after %d successful spins, got %d", expected, successes, bal)
	}
	if bal < 0 {
		t.Fatalf("balance went negative under concurrency: %d", bal)
	}
}

// randHex is crypto/rand-backed (not just for consistency with the
// engine's RNG choice elsewhere — this one's genuinely concurrent-safe,
// which a shared-counter approach would not be, and these IDs are
// generated from multiple goroutines in the concurrency test above).
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // test helper; a broken RNG means the test environment itself is broken
	}
	return hex.EncodeToString(b)
}

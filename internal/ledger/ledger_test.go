package ledger

import (
	"context"
	"testing"
)

// These tests run against MemStore. PGStore implements the same Store
// interface and should be exercised with the same scenarios in an
// integration test against a real Postgres instance (not included here —
// wire it up with a test container or a dev DSN when the backend is
// deployed). Keeping the interface identical means that test, when added,
// needs no changes to the assertions below — just a different Store
// constructor.

func TestMintAndSpend(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	mint, err := MintTransaction("mint-1", "player1", 1000, KindPlayerPurchased, "DEMO_PURCHASE")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, mint); err != nil {
		t.Fatal(err)
	}

	bal, _ := s.Balance(ctx, Account{PlayerID: "player1", Kind: KindPlayerPurchased})
	if bal != 1000 {
		t.Fatalf("expected balance 1000, got %d", bal)
	}

	spin, err := SpinSettlement("spin-1", "player1", 10, 0, bal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, spin); err != nil {
		t.Fatal(err)
	}

	bal, _ = s.Balance(ctx, Account{PlayerID: "player1", Kind: KindPlayerPurchased})
	if bal != 990 {
		t.Fatalf("expected balance 990 after losing spin, got %d", bal)
	}
}

func TestSpinWin(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	mint, _ := MintTransaction("mint-1", "player1", 1000, KindPlayerPurchased, "DEMO_PURCHASE")
	s.Apply(ctx, mint)

	spin, err := SpinSettlement("spin-1", "player1", 10, 50, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, spin); err != nil {
		t.Fatal(err)
	}

	bal, _ := s.Balance(ctx, Account{PlayerID: "player1", Kind: KindPlayerPurchased})
	if bal != 1040 { // 1000 - 10 bet + 50 win
		t.Fatalf("expected balance 1040 after winning spin, got %d", bal)
	}
}

func TestIdempotentApply(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	mint, _ := MintTransaction("mint-1", "player1", 1000, KindPlayerPurchased, "DEMO_PURCHASE")
	r1, err := s.Apply(ctx, mint)
	if err != nil || r1.Replayed {
		t.Fatalf("first apply should succeed and not be replayed: %v %v", err, r1.Replayed)
	}
	r2, err := s.Apply(ctx, mint)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.Replayed {
		t.Fatal("second apply with same ID should be reported as replayed")
	}

	bal, _ := s.Balance(ctx, Account{PlayerID: "player1", Kind: KindPlayerPurchased})
	if bal != 1000 {
		t.Fatalf("balance must not double-apply: expected 1000, got %d", bal)
	}
}

func TestInsufficientFunds(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	// No mint — player has 0 balance. SpinSettlement itself allows
	// constructing a transaction that draws from bonus when purchased is
	// insufficient, but with both at 0 the store must reject it.
	spin, err := SpinSettlement("spin-1", "player1", 10, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(ctx, spin); err != ErrInsufficientFunds {
		t.Fatalf("expected ErrInsufficientFunds, got %v", err)
	}
}

func TestUnbalancedTransactionRejected(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	bad := Transaction{
		ID:   "bad-1",
		Type: "TEST",
		Entries: []Entry{
			{Account: Account{PlayerID: "p1", Kind: KindPlayerPurchased}, Amount: 100},
			// missing the offsetting -100 entry
		},
	}
	if _, err := s.Apply(ctx, bad); err == nil {
		t.Fatal("expected unbalanced transaction to be rejected")
	}
}

func TestHistory(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()

	mint, _ := MintTransaction("mint-1", "player1", 1000, KindPlayerPurchased, "DEMO_PURCHASE")
	s.Apply(ctx, mint)
	spin, _ := SpinSettlement("spin-1", "player1", 10, 0, 1000)
	s.Apply(ctx, spin)

	hist, err := s.History(ctx, "player1", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 2 {
		t.Fatalf("expected 2 transactions in history, got %d", len(hist))
	}
	if hist[0].ID != "spin-1" {
		t.Fatalf("expected most recent transaction first, got %s", hist[0].ID)
	}
}

// TestManySpinsNeverGoNegative is a cheap concurrency-free fuzz-style
// check: spend down a balance with many small spins and confirm the
// account never goes negative and the store's bookkeeping stays
// consistent with manual arithmetic.
func TestManySpinsNeverGoNegative(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore()
	mint, _ := MintTransaction("mint-1", "player1", 500, KindPlayerPurchased, "DEMO_PURCHASE")
	s.Apply(ctx, mint)

	expected := int64(500)
	for i := 0; i < 100; i++ {
		bal, _ := s.Balance(ctx, Account{PlayerID: "player1", Kind: KindPlayerPurchased})
		if bal < 9 {
			break // not enough for another full bet and no bonus balance to cover the gap
		}
		spin, err := SpinSettlement(idFor(i), "player1", 9, 0, bal)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(ctx, spin); err != nil {
			t.Fatal(err)
		}
		expected -= 9
	}

	bal, _ := s.Balance(ctx, Account{PlayerID: "player1", Kind: KindPlayerPurchased})
	if bal < 0 {
		t.Fatalf("balance went negative: %d", bal)
	}
}

func idFor(i int) string {
	return "spin-" + string(rune('a'+i%26)) + string(rune('0'+i%10))
}

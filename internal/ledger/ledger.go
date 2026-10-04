// Package ledger implements a double-entry coin ledger: every movement of
// coins is recorded as a balanced Transaction of two or more Entries whose
// amounts sum to zero. This is the accounting model real-money systems are
// required to use, and adopting it now — while this is still play-money —
// means phase 3 (real-money cash-out) is a config/licensing change, not a
// rewrite.
//
// Core invariant, enforced by every Store implementation:
//
//	For any Transaction, sum(Entries[i].Amount) == 0.
//
// Accounts are logical buckets, not just "one per player". A spin, for
// example, moves coins from the player's PURCHASED or BONUS account into
// the house's WAGER account, and wins move the reverse direction out of a
// PRIZE_POOL account. This keeps player balances auditable per-source,
// which matters both for support disputes and for any future regulator
// asking "where did these coins come from".
package ledger

import (
	"errors"
	"fmt"
	"time"
)

// AccountKind classifies what an account represents. Keeping this an enum
// (not a free-text string) stops typos from creating orphan account types
// that never reconcile.
type AccountKind string

const (
	// KindPlayerPurchased holds coins the player bought with real money
	// (or, pre-payments, coins granted as if purchased for demo purposes).
	KindPlayerPurchased AccountKind = "PLAYER_PURCHASED"
	// KindPlayerBonus holds promotional/free coins (daily bonus, signup
	// grant). Tracked separately from Purchased because real-money systems
	// almost always treat these differently for cash-out eligibility.
	KindPlayerBonus AccountKind = "PLAYER_BONUS"
	// KindHouseWager is where wagered coins land before a spin resolves.
	KindHouseWager AccountKind = "HOUSE_WAGER"
	// KindHousePrizePool is where win payouts are drawn from.
	KindHousePrizePool AccountKind = "HOUSE_PRIZE_POOL"
	// KindSystemMint is the counterparty for creating coins out of thin air
	// (e.g. a demo "purchase" or a daily bonus grant). Every minted coin is
	// traceable to a SystemMint entry, so total coins in the system is
	// always sum(all non-system accounts) and never silently drifts.
	KindSystemMint AccountKind = "SYSTEM_MINT"
)

// Account identifies one ledger account. PlayerID is empty for house/system
// accounts.
type Account struct {
	PlayerID string
	Kind     AccountKind
}

// Key returns a stable map/storage key for an account.
func (a Account) Key() string {
	return fmt.Sprintf("%s:%s", a.PlayerID, a.Kind)
}

// Entry is one leg of a Transaction: a signed amount against one account.
// Positive = credit (increases balance), negative = debit (decreases it).
type Entry struct {
	Account Account
	Amount  int64 // smallest coin unit; never fractional
}

// Transaction is a balanced set of entries plus metadata for audit trails.
type Transaction struct {
	ID        string // caller-supplied idempotency key
	Type      string // e.g. "SPIN_WAGER", "SPIN_PAYOUT", "DEMO_PURCHASE", "DAILY_BONUS"
	Entries   []Entry
	CreatedAt time.Time
	Ref       string // free-form reference, e.g. spin ID
	// Payload is an opaque, caller-defined blob (typically JSON) stored
	// alongside the transaction and returned unchanged on a replayed
	// Apply. This is what makes a replay fully idempotent end-to-end: the
	// API layer stores the serialized SpinResult here so a retried spin
	// request returns the EXACT original grid, not a freshly rolled one
	// tied to an already-settled bet. The ledger package never inspects
	// or interprets this field.
	Payload string
}

var ErrUnbalanced = errors.New("ledger: transaction entries do not sum to zero")
var ErrEmptyTransaction = errors.New("ledger: transaction has no entries")
var ErrDuplicateID = errors.New("ledger: transaction ID already applied")
var ErrInsufficientFunds = errors.New("ledger: insufficient funds")

// Validate checks the double-entry invariant. Every Store.Apply
// implementation MUST call this before touching balances.
func (t Transaction) Validate() error {
	if len(t.Entries) == 0 {
		return ErrEmptyTransaction
	}
	var sum int64
	for _, e := range t.Entries {
		sum += e.Amount
	}
	if sum != 0 {
		return fmt.Errorf("%w: sum=%d", ErrUnbalanced, sum)
	}
	if t.ID == "" {
		return fmt.Errorf("ledger: transaction ID required for idempotency")
	}
	return nil
}

// SpinSettlement builds the balanced transaction for one spin: wager moves
// from the player's purchased+bonus balance (purchased spent first) into
// the house wager account, and any win moves from the prize pool back to
// the player's purchased balance. Keeping this construction in one place
// means the API layer never hand-assembles entries and can't get the
// accounting wrong.
func SpinSettlement(txID, playerID string, bet, win, purchasedBalance int64) (Transaction, error) {
	if bet <= 0 {
		return Transaction{}, fmt.Errorf("ledger: bet must be > 0")
	}
	if win < 0 {
		return Transaction{}, fmt.Errorf("ledger: win must be >= 0")
	}

	fromPurchased := bet
	fromBonus := int64(0)
	if fromPurchased > purchasedBalance {
		fromBonus = fromPurchased - purchasedBalance
		fromPurchased = purchasedBalance
	}

	entries := []Entry{
		{Account: Account{PlayerID: playerID, Kind: KindHouseWager}, Amount: bet},
	}
	if fromPurchased > 0 {
		entries = append(entries, Entry{
			Account: Account{PlayerID: playerID, Kind: KindPlayerPurchased}, Amount: -fromPurchased,
		})
	}
	if fromBonus > 0 {
		entries = append(entries, Entry{
			Account: Account{PlayerID: playerID, Kind: KindPlayerBonus}, Amount: -fromBonus,
		})
	}
	if win > 0 {
		entries = append(entries,
			Entry{Account: Account{PlayerID: playerID, Kind: KindPlayerPurchased}, Amount: win},
			Entry{Account: Account{Kind: KindHousePrizePool}, Amount: -win},
		)
	}
	// Balance the house wager leg against the prize pool so the whole
	// transaction sums to zero: wagered coins conceptually flow into the
	// prize pool as the house's take on losing spins.
	entries = append(entries, Entry{Account: Account{Kind: KindHousePrizePool}, Amount: bet})
	entries = append(entries, Entry{Account: Account{PlayerID: playerID, Kind: KindHouseWager}, Amount: -bet})

	tx := Transaction{ID: txID, Type: "SPIN_SETTLEMENT", Entries: entries, Ref: playerID}
	if err := tx.Validate(); err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

// MintTransaction creates coins for a player (demo "purchase" or bonus
// grant), balanced against the SYSTEM_MINT account. In the real-money
// phase, a real purchase's mint is tied 1:1 to a verified payment
// provider webhook — never trust a client to request its own mint.
func MintTransaction(txID, playerID string, amount int64, kind AccountKind, txType string) (Transaction, error) {
	if amount <= 0 {
		return Transaction{}, fmt.Errorf("ledger: mint amount must be > 0")
	}
	if kind != KindPlayerPurchased && kind != KindPlayerBonus {
		return Transaction{}, fmt.Errorf("ledger: mint target must be a player account")
	}
	tx := Transaction{
		ID:   txID,
		Type: txType,
		Entries: []Entry{
			{Account: Account{PlayerID: playerID, Kind: kind}, Amount: amount},
			{Account: Account{Kind: KindSystemMint}, Amount: -amount},
		},
		Ref: playerID,
	}
	if err := tx.Validate(); err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

// MintPackage creates the coins for a store package in one balanced
// transaction: base coins into PLAYER_PURCHASED and any bonus coins into
// PLAYER_BONUS, both offset against SYSTEM_MINT. In the real-money phase
// this is only ever called from a verified payment-provider webhook.
func MintPackage(txID, playerID string, coins, bonus int64, txType string) (Transaction, error) {
	if coins <= 0 || bonus < 0 {
		return Transaction{}, fmt.Errorf("ledger: invalid package amounts")
	}
	entries := []Entry{
		{Account: Account{PlayerID: playerID, Kind: KindPlayerPurchased}, Amount: coins},
	}
	if bonus > 0 {
		entries = append(entries, Entry{Account: Account{PlayerID: playerID, Kind: KindPlayerBonus}, Amount: bonus})
	}
	entries = append(entries, Entry{Account: Account{Kind: KindSystemMint}, Amount: -(coins + bonus)})
	tx := Transaction{ID: txID, Type: txType, Entries: entries, Ref: playerID}
	if err := tx.Validate(); err != nil {
		return Transaction{}, err
	}
	return tx, nil
}

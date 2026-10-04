package api

import (
	"net/http"
	"strconv"

	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

// GameInfo describes a game to a client: everything needed to render the
// reels, highlight winning lines, and show a paytable. The server stays
// the source of truth; clients never hardcode payouts.
type GameInfo struct {
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Cols          int                       `json:"cols"`
	Rows          int                       `json:"rows"`
	Lines         int                       `json:"lines"`
	Paylines      [][]int                   `json:"paylines"`
	Wild          string                    `json:"wild"`
	Paytable      map[string]map[string]int `json:"paytable"`
	ScatterPays   map[string]int            `json:"scatterPays"`
	MinBetPerLine int64                     `json:"minBetPerLine"`
	MaxBetPerLine int64                     `json:"maxBetPerLine"`
}

func gameInfo(cfg engine.Config) GameInfo {
	info := GameInfo{
		ID: cfg.ID, Name: cfg.Name,
		Cols: len(cfg.Reels), Rows: cfg.Rows, Lines: len(cfg.Paylines),
		Wild:          string(cfg.WildSymbol),
		Paytable:      map[string]map[string]int{},
		ScatterPays:   map[string]int{},
		MinBetPerLine: engine.MinBetPerLine,
		MaxBetPerLine: engine.MaxBetPerLine,
	}
	for _, pl := range cfg.Paylines {
		info.Paylines = append(info.Paylines, []int(pl))
	}
	for sym, rule := range cfg.Paytable {
		m := map[string]int{}
		for count, mult := range rule {
			m[strconv.Itoa(count)] = mult
		}
		info.Paytable[string(sym)] = m
	}
	for count, mult := range cfg.ScatterPays {
		info.ScatterPays[strconv.Itoa(count)] = mult
	}
	return info
}

func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	out := make([]GameInfo, 0, len(s.GameOrder))
	for _, id := range s.GameOrder {
		out = append(out, gameInfo(s.Engines[id].Config()))
	}
	writeJSON(w, http.StatusOK, map[string]any{"games": out})
}

// Package is a coin pack in the (demo) store.
type Package struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Coins      int64  `json:"coins"`
	Bonus      int64  `json:"bonus"`
	PriceLabel string `json:"priceLabel"`
}

// Packages is the store catalog. Prices are labels only: this build takes
// NO payment (see handleDemoPurchase).
var Packages = []Package{
	{ID: "starter", Name: "Starter Stash", Coins: 1000, Bonus: 0, PriceLabel: "$0.99"},
	{ID: "popular", Name: "Lucky Pile", Coins: 5500, Bonus: 500, PriceLabel: "$4.99"},
	{ID: "chest", Name: "Treasure Chest", Coins: 12000, Bonus: 2000, PriceLabel: "$9.99"},
	{ID: "vault", Name: "Mega Vault", Coins: 32000, Bonus: 8000, PriceLabel: "$19.99"},
}

func (s *Server) handlePackages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"demo": true, "packages": Packages})
}

type demoPurchaseBody struct {
	PackageID string `json:"packageId"`
	Amount    int64  `json:"amount"` // dev/testing escape hatch; ignored if packageId is set
}

// handleDemoPurchase grants coins with NO real payment. This endpoint must
// not exist in a build with real-money cash-out enabled: when payments are
// wired up, minting happens ONLY from a verified payment-provider webhook,
// never from a client-initiated request like this one.
func (s *Server) handleDemoPurchase(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")
	var body demoPurchaseBody
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	txID, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate transaction id")
		return
	}

	var tx ledger.Transaction
	if body.PackageID != "" {
		var pkg *Package
		for i := range Packages {
			if Packages[i].ID == body.PackageID {
				pkg = &Packages[i]
			}
		}
		if pkg == nil {
			writeError(w, http.StatusBadRequest, "unknown package")
			return
		}
		tx, err = ledger.MintPackage(txID, playerID, pkg.Coins, pkg.Bonus, "DEMO_PURCHASE")
	} else {
		if body.Amount <= 0 || body.Amount > 1_000_000 {
			writeError(w, http.StatusBadRequest, "invalid amount")
			return
		}
		tx, err = ledger.MintTransaction(txID, playerID, body.Amount, ledger.KindPlayerPurchased, "DEMO_PURCHASE")
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mint construction failed")
		return
	}
	if _, err := s.Store.Apply(r.Context(), tx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to apply purchase")
		return
	}

	balances, _ := s.Store.Balances(r.Context(), playerID)
	writeJSON(w, http.StatusOK, map[string]int64{
		"purchased": balances[ledger.KindPlayerPurchased],
		"bonus":     balances[ledger.KindPlayerBonus],
	})
}

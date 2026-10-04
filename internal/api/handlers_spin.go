package api

import (
	"encoding/json"
	"net/http"

	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

// spinRequestBody carries INTENT only (which game, how much). The server
// computes everything else — see the engine package doc for why.
type spinRequestBody struct {
	GameID      string `json:"gameId"`
	BetPerLine  int64  `json:"betPerLine"`
	LinesPlayed int    `json:"linesPlayed"`
	// IdempotencyKey lets a client safely retry a timed-out spin without a
	// double charge. Required.
	IdempotencyKey string `json:"idempotencyKey"`
}

type spinResponseBody struct {
	GameID           string            `json:"gameId"`
	GameVersion      string            `json:"gameVersion"`
	Grid             [][]engine.Symbol `json:"grid"`
	LineWins         []engine.LineWin  `json:"lineWins"`
	ScatterWin       int64             `json:"scatterWin"`
	TotalWin         int64             `json:"totalWin"`
	TotalBet         int64             `json:"totalBet"`
	PurchasedBalance int64             `json:"purchasedBalance"`
	BonusBalance     int64             `json:"bonusBalance"`
	Replayed         bool              `json:"replayed"`
}

func (s *Server) handleSpin(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")

	var body spinRequestBody
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotencyKey required")
		return
	}
	if body.GameID == "" {
		body.GameID = s.GameOrder[0]
	}
	eng, ok := s.Engines[body.GameID]
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown game")
		return
	}
	if body.BetPerLine < engine.MinBetPerLine || body.BetPerLine > engine.MaxBetPerLine {
		writeError(w, http.StatusBadRequest, "bet out of range")
		return
	}
	if body.LinesPlayed <= 0 || body.LinesPlayed > len(eng.Config().Paylines) {
		writeError(w, http.StatusBadRequest, "invalid lines played")
		return
	}

	ctx := r.Context()

	// Idempotency FIRST: a retried request returns the exact original
	// spin without consulting the RNG.
	if existing, found, err := s.Store.Get(ctx, body.IdempotencyKey); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check idempotency")
		return
	} else if found {
		var cached spinResponseBody
		if err := json.Unmarshal([]byte(existing.Payload), &cached); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to decode cached spin result")
			return
		}
		balances, err := s.Store.Balances(ctx, playerID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load balance")
			return
		}
		cached.PurchasedBalance = balances[ledger.KindPlayerPurchased]
		cached.BonusBalance = balances[ledger.KindPlayerBonus]
		cached.Replayed = true
		writeJSON(w, http.StatusOK, cached)
		return
	}

	balances, err := s.Store.Balances(ctx, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load balance")
		return
	}
	available := balances[ledger.KindPlayerPurchased] + balances[ledger.KindPlayerBonus]
	if body.BetPerLine*int64(body.LinesPlayed) > available {
		writeError(w, http.StatusPaymentRequired, "insufficient balance")
		return
	}

	result, err := eng.Spin(engine.SpinRequest{
		SessionID:   playerID,
		BetPerLine:  body.BetPerLine,
		LinesPlayed: body.LinesPlayed,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	settlement, err := ledger.SpinSettlement(
		body.IdempotencyKey, playerID, result.TotalBet, result.TotalWin, balances[ledger.KindPlayerPurchased],
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "settlement construction failed")
		return
	}

	respBody := spinResponseBody{
		GameID:      result.GameID,
		GameVersion: result.GameVersion,
		Grid:        result.Grid,
		LineWins:    result.LineWins,
		ScatterWin:  result.ScatterWin,
		TotalWin:    result.TotalWin,
		TotalBet:    result.TotalBet,
	}
	payload, err := json.Marshal(respBody)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to encode spin result")
		return
	}
	settlement.Payload = string(payload)

	applyResult, err := s.Store.Apply(ctx, settlement)
	if err != nil {
		if err == ledger.ErrInsufficientFunds {
			writeError(w, http.StatusPaymentRequired, "insufficient balance")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to settle spin")
		return
	}

	newBalances, err := s.Store.Balances(ctx, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load updated balance")
		return
	}
	respBody.PurchasedBalance = newBalances[ledger.KindPlayerPurchased]
	respBody.BonusBalance = newBalances[ledger.KindPlayerBonus]
	respBody.Replayed = applyResult.Replayed
	writeJSON(w, http.StatusOK, respBody)
}

// Package api exposes the game engine and ledger over HTTP for a mobile
// client. Everything here assumes an already-authenticated player
// (auth middleware is a documented stub — see Server.authenticate — and
// MUST be replaced before this ever leaves local dev).
//
// Every handler here is deliberately dumb: validate input, call into
// engine/ledger, serialize the result. All game logic and money logic
// lives in internal/engine and internal/ledger so it stays unit-testable
// without spinning up HTTP at all.
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

// Server holds the shared dependencies for all handlers.
type Server struct {
	Engine *engine.Engine
	Store  ledger.Store
	Mux    *http.ServeMux
}

func NewServer(eng *engine.Engine, store ledger.Store) *Server {
	s := &Server{Engine: eng, Store: store, Mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) routes() {
	// Go 1.22's ServeMux supports method + path-pattern matching natively,
	// so no third-party router is needed — one less dependency to vet and
	// keep updated, and one less thing to port if the backend language
	// ever changes.
	s.Mux.HandleFunc("POST /v1/players/{playerID}/spin", s.handleSpin)
	s.Mux.HandleFunc("GET /v1/players/{playerID}/balance", s.handleBalance)
	s.Mux.HandleFunc("POST /v1/players/{playerID}/demo-purchase", s.handleDemoPurchase)
	s.Mux.HandleFunc("GET /v1/players/{playerID}/history", s.handleHistory)
	s.Mux.HandleFunc("GET /v1/healthz", s.handleHealthz)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "gameVersion": engine.GameVersion})
}

// spinRequestBody is the client-facing request shape. Note it carries NO
// outcome data — only intent (how much to bet, how many lines). The
// server computes everything else; see the engine package doc comment
// for why that's non-negotiable.
type spinRequestBody struct {
	BetPerLine  int64 `json:"betPerLine"`
	LinesPlayed int   `json:"linesPlayed"`
	// IdempotencyKey lets a client safely retry a spin request that timed
	// out without risking a double-charge. Required.
	IdempotencyKey string `json:"idempotencyKey"`
}

type spinResponseBody struct {
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
	if playerID == "" {
		writeError(w, http.StatusBadRequest, "playerID required")
		return
	}

	var body spinRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.IdempotencyKey == "" {
		writeError(w, http.StatusBadRequest, "idempotencyKey required")
		return
	}

	ctx := r.Context()

	balances, err := s.Store.Balances(ctx, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load balance")
		return
	}
	available := balances[ledger.KindPlayerPurchased] + balances[ledger.KindPlayerBonus]
	wantBet := body.BetPerLine * int64(body.LinesPlayed)
	if wantBet <= 0 {
		writeError(w, http.StatusBadRequest, "bet must be > 0")
		return
	}
	if wantBet > available {
		writeError(w, http.StatusPaymentRequired, "insufficient balance")
		return
	}

	// The spin outcome itself is NOT idempotent-replayable the same way
	// the ledger write is: if this handler crashes after Spin() but
	// before Apply(), a client retry with the same IdempotencyKey would
	// get a fresh spin outcome tied to the same ledger entry it already
	// has. For a production system, persist the SpinResult alongside the
	// ledger transaction (same DB transaction) so a true replay returns
	// the original grid too. Flagged here rather than silently shipped as
	// "idempotent" when it is only partially so.
	result, err := s.Engine.Spin(engine.SpinRequest{
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

	writeJSON(w, http.StatusOK, spinResponseBody{
		GameVersion:      result.GameVersion,
		Grid:             result.Grid,
		LineWins:         result.LineWins,
		ScatterWin:       result.ScatterWin,
		TotalWin:         result.TotalWin,
		TotalBet:         result.TotalBet,
		PurchasedBalance: newBalances[ledger.KindPlayerPurchased],
		BonusBalance:     newBalances[ledger.KindPlayerBonus],
		Replayed:         applyResult.Replayed,
	})
}

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")
	balances, err := s.Store.Balances(r.Context(), playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load balance")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{
		"purchased": balances[ledger.KindPlayerPurchased],
		"bonus":     balances[ledger.KindPlayerBonus],
	})
}

type demoPurchaseBody struct {
	Amount int64 `json:"amount"`
}

// handleDemoPurchase grants coins with NO real payment involved — this
// endpoint must never exist in a build with real-money cash-out enabled.
// When real payments are wired up, coin minting happens ONLY from a
// verified payment-provider webhook handler, never from a
// client-initiated request like this one.
func (s *Server) handleDemoPurchase(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")
	var body demoPurchaseBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Amount <= 0 {
		writeError(w, http.StatusBadRequest, "invalid amount")
		return
	}

	txID, err := newIdempotencyKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate transaction id")
		return
	}

	tx, err := ledger.MintTransaction(txID, playerID, body.Amount, ledger.KindPlayerPurchased, "DEMO_PURCHASE")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "mint construction failed")
		return
	}
	if _, err := s.Store.Apply(r.Context(), tx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to apply purchase")
		return
	}

	balances, _ := s.Store.Balances(r.Context(), playerID)
	writeJSON(w, http.StatusOK, map[string]int64{"purchased": balances[ledger.KindPlayerPurchased]})
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")
	hist, err := s.Store.History(r.Context(), playerID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load history")
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

func newIdempotencyKey() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: failed to encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// loggingMiddleware is a minimal request logger. Replace with structured
// logging (and request IDs, and auth context) before this is anything
// more than a local demo.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

package api

import (
	"net/http"

	"slots-engine/internal/ledger"
)

// WelcomeBonusCoins is granted (to the bonus balance) on guest sign-up.
const WelcomeBonusCoins int64 = 1000

type guestResponse struct {
	PlayerID     string `json:"playerId"`
	Token        string `json:"token"`
	WelcomeBonus int64  `json:"welcomeBonus"`
}

// handleGuest creates an anonymous guest account: a random player ID plus a
// signed bearer token, and a welcome bonus. Guest play is the standard
// social-casino on-ramp; linking to Apple/Google sign-in later means
// attaching an identity to this player ID, not replacing it. Rate-limited
// per IP by the router.
func (s *Server) handleGuest(w http.ResponseWriter, r *http.Request) {
	if s.Tokens == nil {
		writeError(w, http.StatusNotImplemented, "guest accounts are disabled")
		return
	}
	id, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	tx, err := ledger.MintTransaction("welcome-"+id, id, WelcomeBonusCoins, ledger.KindPlayerBonus, "WELCOME_BONUS")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create welcome bonus")
		return
	}
	if _, err := s.Store.Apply(r.Context(), tx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create account")
		return
	}
	writeJSON(w, http.StatusCreated, guestResponse{
		PlayerID:     id,
		Token:        s.Tokens.Issue(id),
		WelcomeBonus: WelcomeBonusCoins,
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

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")
	hist, err := s.Store.History(r.Context(), playerID, 50)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load history")
		return
	}
	writeJSON(w, http.StatusOK, hist)
}

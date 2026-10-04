package api

import (
	"encoding/json"
	"net/http"
	"time"

	"slots-engine/internal/ledger"
)

// DailyRewards is the 7-day bonus ladder. After day 7 the cycle restarts.
// Missing a day resets the player to day 1.
var DailyRewards = []int64{500, 750, 1000, 1500, 2000, 3000, 5000}

type dailyPayload struct {
	Day    int   `json:"day"`
	Amount int64 `json:"amount"`
}

type dailyStatus struct {
	Day         int     `json:"day"`         // 1..7: which ladder step applies today
	Reward      int64   `json:"reward"`      // coins for today's step
	Claimed     bool    `json:"claimed"`     // already claimed today (UTC)
	Rewards     []int64 `json:"rewards"`     // the full ladder, for display
	NextClaimAt string  `json:"nextClaimAt"` // start of next UTC day, RFC3339
	Replayed    bool    `json:"replayed,omitempty"`
}

func dailyTxID(playerID string, t time.Time) string {
	return "daily-" + playerID + "-" + t.UTC().Format("20060102")
}

// dailyState derives today's ladder step with O(1) lookups: the claim made
// yesterday (if any) records which step it was, so today's step follows it.
func (s *Server) dailyState(r *http.Request, playerID string) (dailyStatus, error) {
	now := s.Now().UTC()
	ctx := r.Context()

	st := dailyStatus{Rewards: DailyRewards}
	st.NextClaimAt = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)

	if tx, ok, err := s.Store.Get(ctx, dailyTxID(playerID, now)); err != nil {
		return st, err
	} else if ok {
		var p dailyPayload
		if json.Unmarshal([]byte(tx.Payload), &p) == nil && p.Day >= 1 {
			st.Claimed = true
			st.Day = p.Day
			st.Reward = p.Amount
			return st, nil
		}
	}

	st.Day = 1
	if tx, ok, err := s.Store.Get(ctx, dailyTxID(playerID, now.AddDate(0, 0, -1))); err != nil {
		return st, err
	} else if ok {
		var p dailyPayload
		if json.Unmarshal([]byte(tx.Payload), &p) == nil && p.Day >= 1 {
			st.Day = p.Day%len(DailyRewards) + 1
		}
	}
	st.Reward = DailyRewards[st.Day-1]
	return st, nil
}

func (s *Server) handleDailyStatus(w http.ResponseWriter, r *http.Request) {
	st, err := s.dailyState(r, r.PathValue("playerID"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load daily bonus")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleDailyClaim is naturally idempotent: the transaction ID encodes
// player + UTC date, so a double-tap or retry can never pay twice.
func (s *Server) handleDailyClaim(w http.ResponseWriter, r *http.Request) {
	playerID := r.PathValue("playerID")
	st, err := s.dailyState(r, playerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load daily bonus")
		return
	}
	if st.Claimed {
		st.Replayed = true
		writeJSON(w, http.StatusOK, st)
		return
	}

	tx, err := ledger.MintTransaction(dailyTxID(playerID, s.Now()), playerID, st.Reward, ledger.KindPlayerBonus, "DAILY_BONUS")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to build bonus")
		return
	}
	payload, _ := json.Marshal(dailyPayload{Day: st.Day, Amount: st.Reward})
	tx.Payload = string(payload)

	res, err := s.Store.Apply(r.Context(), tx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to grant bonus")
		return
	}
	st.Claimed = true
	st.Replayed = res.Replayed
	writeJSON(w, http.StatusOK, st)
}

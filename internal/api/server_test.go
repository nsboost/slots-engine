package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

func newTestServer(t *testing.T) (*Server, string, string) {
	t.Helper()
	eng, err := engine.New(engine.DemoFortuneReels())
	if err != nil {
		t.Fatal(err)
	}
	store := ledger.NewMemStore()
	auth := NewStaticKeyAuthenticator(map[string]string{"alice": "alice-key", "bob": "bob-key"})
	rl := NewRateLimiter(1000, time.Minute) // high limit by default; specific tests override
	srv := NewServer(eng, store, auth, rl)
	return srv, "alice", "alice-key"
}

func doRequest(srv *Server, method, path, key string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if key != "" {
		req.Header.Set("X-Player-Key", key)
	}
	w := httptest.NewRecorder()
	srv.Mux.ServeHTTP(w, req)
	return w
}

func TestHealthzUnauthenticated(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv, "GET", "/v1/healthz", "", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSpinRequiresAuth(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv, "POST", "/v1/players/alice/spin", "", map[string]any{
		"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "k1",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with no key, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSpinRejectsWrongKey(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv, "POST", "/v1/players/alice/spin", "wrong-key", map[string]any{
		"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "k1",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d", w.Code)
	}
}

// TestCannotActOnAnotherPlayer is the important security case: bob's
// valid key must not let him touch alice's account just by changing the
// URL path.
func TestCannotActOnAnotherPlayer(t *testing.T) {
	srv, _, _ := newTestServer(t)
	w := doRequest(srv, "GET", "/v1/players/alice/balance", "bob-key", nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when bob's key targets alice's path, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDemoPurchaseThenSpin(t *testing.T) {
	srv, player, key := newTestServer(t)

	w := doRequest(srv, "POST", "/v1/players/"+player+"/demo-purchase", key, map[string]any{"amount": 1000})
	if w.Code != http.StatusOK {
		t.Fatalf("purchase failed: %d %s", w.Code, w.Body.String())
	}

	w = doRequest(srv, "POST", "/v1/players/"+player+"/spin", key, map[string]any{
		"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "spin-1",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("spin failed: %d %s", w.Code, w.Body.String())
	}
	var resp spinResponseBody
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.TotalBet != 9 {
		t.Fatalf("expected bet 9, got %d", resp.TotalBet)
	}
	if resp.Replayed {
		t.Fatal("first spin should not be marked replayed")
	}
}

// TestSpinReplayReturnsIdenticalGrid is the regression test for the gap
// closed this session: a retried spin request must return the EXACT same
// grid, not a freshly rolled one.
func TestSpinReplayReturnsIdenticalGrid(t *testing.T) {
	srv, player, key := newTestServer(t)
	doRequest(srv, "POST", "/v1/players/"+player+"/demo-purchase", key, map[string]any{"amount": 1000})

	spinBody := map[string]any{"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "spin-replay-1"}

	w1 := doRequest(srv, "POST", "/v1/players/"+player+"/spin", key, spinBody)
	var r1 spinResponseBody
	json.NewDecoder(w1.Body).Decode(&r1)

	w2 := doRequest(srv, "POST", "/v1/players/"+player+"/spin", key, spinBody)
	var r2 spinResponseBody
	json.NewDecoder(w2.Body).Decode(&r2)

	if !r2.Replayed {
		t.Fatal("second identical request should be marked replayed")
	}
	if len(r1.Grid) != len(r2.Grid) {
		t.Fatalf("grid column count differs between original and replay")
	}
	for c := range r1.Grid {
		for row := range r1.Grid[c] {
			if r1.Grid[c][row] != r2.Grid[c][row] {
				t.Fatalf("replay grid differs from original at col %d row %d: %s vs %s", c, row, r1.Grid[c][row], r2.Grid[c][row])
			}
		}
	}
	if r1.TotalWin != r2.TotalWin || r1.TotalBet != r2.TotalBet {
		t.Fatalf("replay win/bet differ from original: (%d,%d) vs (%d,%d)", r1.TotalBet, r1.TotalWin, r2.TotalBet, r2.TotalWin)
	}
}

// TestSpinReplayDoesNotDoubleCharge confirms the balance after a replay
// matches the balance after the original spin, not double the bet.
func TestSpinReplayDoesNotDoubleCharge(t *testing.T) {
	srv, player, key := newTestServer(t)
	doRequest(srv, "POST", "/v1/players/"+player+"/demo-purchase", key, map[string]any{"amount": 1000})

	spinBody := map[string]any{"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "spin-replay-2"}
	doRequest(srv, "POST", "/v1/players/"+player+"/spin", key, spinBody)

	w := doRequest(srv, "GET", "/v1/players/"+player+"/balance", key, nil)
	var balAfterFirst map[string]int64
	json.NewDecoder(w.Body).Decode(&balAfterFirst)

	doRequest(srv, "POST", "/v1/players/"+player+"/spin", key, spinBody) // replay

	w = doRequest(srv, "GET", "/v1/players/"+player+"/balance", key, nil)
	var balAfterReplay map[string]int64
	json.NewDecoder(w.Body).Decode(&balAfterReplay)

	if balAfterFirst["purchased"] != balAfterReplay["purchased"] {
		t.Fatalf("replay changed balance: %d -> %d", balAfterFirst["purchased"], balAfterReplay["purchased"])
	}
}

func TestInsufficientBalanceRejected(t *testing.T) {
	srv, player, key := newTestServer(t)
	// no purchase made — balance is 0
	w := doRequest(srv, "POST", "/v1/players/"+player+"/spin", key, map[string]any{
		"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "spin-1",
	})
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402 for insufficient balance, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRateLimiting(t *testing.T) {
	eng, _ := engine.New(engine.DemoFortuneReels())
	store := ledger.NewMemStore()
	auth := NewStaticKeyAuthenticator(map[string]string{"alice": "alice-key"})
	rl := NewRateLimiter(3, time.Minute) // deliberately tight for this test
	srv := NewServer(eng, store, auth, rl)

	var lastCode int
	for i := 0; i < 5; i++ {
		w := doRequest(srv, "GET", "/v1/players/alice/balance", "alice-key", nil)
		lastCode = w.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after exceeding limit, got %d", lastCode)
	}
}

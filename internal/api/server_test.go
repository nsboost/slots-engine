package api

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

type testEnv struct {
	srv    *Server
	tokens *TokenAuthenticator
	clock  *time.Time
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	var engines []*engine.Engine
	for _, cfg := range engine.AllGames() {
		e, err := engine.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		engines = append(engines, e)
	}
	tokens := NewTokenAuthenticator([]byte("test-secret-0123456789"))
	start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	env := &testEnv{tokens: tokens, clock: &start}
	srv, err := NewServer(Config{
		Engines: engines,
		Store:   ledger.NewMemStore(),
		Auth: ChainAuthenticator{
			tokens,
			NewStaticKeyAuthenticator(map[string]string{"alice": "alice-key", "bob": "bob-key"}),
		},
		Tokens:       tokens,
		RateLimiter:  NewRateLimiter(100000, time.Minute),
		GuestLimiter: NewRateLimiter(100000, time.Hour),
		Now:          func() time.Time { return *env.clock },
	})
	if err != nil {
		t.Fatal(err)
	}
	env.srv = srv
	return env
}

func (e *testEnv) do(method, path string, headers map[string]string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	e.srv.Mux.ServeHTTP(w, req)
	return w
}

func alice() map[string]string { return map[string]string{"X-Player-Key": "alice-key"} }

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.NewDecoder(w.Body).Decode(&v); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return v
}

func (e *testEnv) fund(t *testing.T, amount int) {
	t.Helper()
	w := e.do("POST", "/v1/players/alice/demo-purchase", alice(), map[string]any{"amount": amount})
	if w.Code != http.StatusOK {
		t.Fatalf("fund failed: %d %s", w.Code, w.Body.String())
	}
}

func TestHealthzUnauthenticated(t *testing.T) {
	e := newEnv(t)
	if w := e.do("GET", "/v1/healthz", nil, nil); w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestAuthFailures(t *testing.T) {
	e := newEnv(t)
	spin := map[string]any{"betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "k1"}
	if w := e.do("POST", "/v1/players/alice/spin", nil, spin); w.Code != http.StatusUnauthorized {
		t.Fatalf("no key: want 401 got %d", w.Code)
	}
	if w := e.do("POST", "/v1/players/alice/spin", map[string]string{"X-Player-Key": "nope"}, spin); w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong key: want 401 got %d", w.Code)
	}
	// bob's valid key against alice's path must be 403, not 401.
	if w := e.do("GET", "/v1/players/alice/balance", map[string]string{"X-Player-Key": "bob-key"}, nil); w.Code != http.StatusForbidden {
		t.Fatalf("cross-player: want 403 got %d", w.Code)
	}
}

func TestGuestRegistrationAndTokenAuth(t *testing.T) {
	e := newEnv(t)
	w := e.do("POST", "/v1/auth/guest", nil, nil)
	if w.Code != http.StatusCreated {
		t.Fatalf("guest: want 201 got %d: %s", w.Code, w.Body.String())
	}
	g := decode[guestResponse](t, w)
	if len(g.PlayerID) != 32 || g.Token == "" || g.WelcomeBonus != WelcomeBonusCoins {
		t.Fatalf("unexpected guest response: %+v", g)
	}
	h := map[string]string{"Authorization": "Bearer " + g.Token}

	w = e.do("GET", "/v1/players/"+g.PlayerID+"/balance", h, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("balance: %d %s", w.Code, w.Body.String())
	}
	bal := decode[map[string]int64](t, w)
	if bal["bonus"] != WelcomeBonusCoins || bal["purchased"] != 0 {
		t.Fatalf("welcome bonus not credited to bonus balance: %v", bal)
	}

	// Welcome-bonus coins must be spendable.
	w = e.do("POST", "/v1/players/"+g.PlayerID+"/spin", h, map[string]any{
		"gameId": "fortune-reels", "betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "g-spin-1"})
	if w.Code != http.StatusOK {
		t.Fatalf("spin with bonus coins failed: %d %s", w.Code, w.Body.String())
	}
}

func TestTokenForgeryRejected(t *testing.T) {
	e := newEnv(t)
	g := decode[guestResponse](t, e.do("POST", "/v1/auth/guest", nil, nil))
	path := "/v1/players/" + g.PlayerID + "/balance"

	tampered := g.Token[:len(g.Token)-1] + "0"
	if g.Token[len(g.Token)-1] == '0' {
		tampered = g.Token[:len(g.Token)-1] + "1"
	}
	if w := e.do("GET", path, map[string]string{"Authorization": "Bearer " + tampered}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("tampered signature: want 401 got %d", w.Code)
	}
	// A token signed with a different secret must fail.
	other := NewTokenAuthenticator([]byte("another-secret-xxxxxxxx")).Issue(g.PlayerID)
	if w := e.do("GET", path, map[string]string{"Authorization": "Bearer " + other}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("foreign secret: want 401 got %d", w.Code)
	}
	// A valid token for guest A must not open guest B's account.
	g2 := decode[guestResponse](t, e.do("POST", "/v1/auth/guest", nil, nil))
	if w := e.do("GET", "/v1/players/"+g2.PlayerID+"/balance", map[string]string{"Authorization": "Bearer " + g.Token}, nil); w.Code != http.StatusForbidden {
		t.Fatalf("cross-guest: want 403 got %d", w.Code)
	}
}

func TestGamesCatalog(t *testing.T) {
	e := newEnv(t)
	w := e.do("GET", "/v1/games", nil, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("games: %d", w.Code)
	}
	resp := decode[struct{ Games []GameInfo }](t, w)
	if len(resp.Games) != 2 {
		t.Fatalf("want 2 games, got %d", len(resp.Games))
	}
	for _, g := range resp.Games {
		if g.Cols != 5 || g.Rows != 3 || g.Lines != len(g.Paylines) || g.Lines == 0 {
			t.Fatalf("bad game info: %+v", g)
		}
		if len(g.Paytable) == 0 || len(g.ScatterPays) == 0 || g.Wild == "" {
			t.Fatalf("missing paytable data: %+v", g)
		}
	}
	if resp.Games[0].Lines != 9 || resp.Games[1].Lines != 15 {
		t.Fatalf("unexpected line counts: %d, %d", resp.Games[0].Lines, resp.Games[1].Lines)
	}
}

func TestSpinBothGamesAndValidation(t *testing.T) {
	e := newEnv(t)
	e.fund(t, 100000)
	for i, c := range []struct {
		game  string
		lines int
	}{{"fortune-reels", 9}, {"gold-rush", 15}} {
		w := e.do("POST", "/v1/players/alice/spin", alice(), map[string]any{
			"gameId": c.game, "betPerLine": 2, "linesPlayed": c.lines, "idempotencyKey": "s" + string(rune('a'+i))})
		if w.Code != http.StatusOK {
			t.Fatalf("%s spin: %d %s", c.game, w.Code, w.Body.String())
		}
		r := decode[spinResponseBody](t, w)
		if r.GameID != c.game || r.TotalBet != int64(2*c.lines) || len(r.Grid) != 5 {
			t.Fatalf("%s: bad response %+v", c.game, r)
		}
	}
	bad := []map[string]any{
		{"gameId": "nope", "betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "b1"},
		{"gameId": "fortune-reels", "betPerLine": 0, "linesPlayed": 9, "idempotencyKey": "b2"},
		{"gameId": "fortune-reels", "betPerLine": 101, "linesPlayed": 9, "idempotencyKey": "b3"},
		{"gameId": "fortune-reels", "betPerLine": 1, "linesPlayed": 10, "idempotencyKey": "b4"},
		{"gameId": "fortune-reels", "betPerLine": 1, "linesPlayed": 9},
	}
	for i, b := range bad {
		if w := e.do("POST", "/v1/players/alice/spin", alice(), b); w.Code != http.StatusBadRequest {
			t.Fatalf("bad[%d]: want 400 got %d (%s)", i, w.Code, w.Body.String())
		}
	}
}

func TestSpinReplayReturnsIdenticalGridAndNoDoubleCharge(t *testing.T) {
	e := newEnv(t)
	e.fund(t, 1000)
	body := map[string]any{"gameId": "gold-rush", "betPerLine": 1, "linesPlayed": 15, "idempotencyKey": "replay-1"}

	r1 := decode[spinResponseBody](t, e.do("POST", "/v1/players/alice/spin", alice(), body))
	bal1 := decode[map[string]int64](t, e.do("GET", "/v1/players/alice/balance", alice(), nil))
	r2 := decode[spinResponseBody](t, e.do("POST", "/v1/players/alice/spin", alice(), body))
	bal2 := decode[map[string]int64](t, e.do("GET", "/v1/players/alice/balance", alice(), nil))

	if r1.Replayed || !r2.Replayed {
		t.Fatalf("replayed flags wrong: %v %v", r1.Replayed, r2.Replayed)
	}
	for c := range r1.Grid {
		for row := range r1.Grid[c] {
			if r1.Grid[c][row] != r2.Grid[c][row] {
				t.Fatalf("replay grid differs at %d,%d", c, row)
			}
		}
	}
	if r1.TotalWin != r2.TotalWin || bal1["purchased"] != bal2["purchased"] {
		t.Fatalf("replay changed outcome or balance: %v vs %v", bal1, bal2)
	}
}

func TestInsufficientBalanceRejected(t *testing.T) {
	e := newEnv(t)
	w := e.do("POST", "/v1/players/alice/spin", alice(), map[string]any{
		"gameId": "fortune-reels", "betPerLine": 1, "linesPlayed": 9, "idempotencyKey": "k"})
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("want 402 got %d", w.Code)
	}
}

func TestStorePackages(t *testing.T) {
	e := newEnv(t)
	w := e.do("GET", "/v1/store/packages", nil, nil)
	resp := decode[struct {
		Demo     bool
		Packages []Package
	}](t, w)
	if !resp.Demo || len(resp.Packages) != 4 {
		t.Fatalf("bad catalog: %+v", resp)
	}
	w = e.do("POST", "/v1/players/alice/demo-purchase", alice(), map[string]any{"packageId": "popular"})
	if w.Code != http.StatusOK {
		t.Fatalf("purchase: %d %s", w.Code, w.Body.String())
	}
	bal := decode[map[string]int64](t, w)
	if bal["purchased"] != 5500 || bal["bonus"] != 500 {
		t.Fatalf("package credited wrong: %v", bal)
	}
	if w := e.do("POST", "/v1/players/alice/demo-purchase", alice(), map[string]any{"packageId": "nope"}); w.Code != http.StatusBadRequest {
		t.Fatalf("unknown package: want 400 got %d", w.Code)
	}
}

func TestDailyBonusLadder(t *testing.T) {
	e := newEnv(t)
	claim := func() dailyStatus {
		w := e.do("POST", "/v1/players/alice/daily-bonus", alice(), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("claim: %d %s", w.Code, w.Body.String())
		}
		return decode[dailyStatus](t, w)
	}
	bonus := func() int64 {
		return decode[map[string]int64](t, e.do("GET", "/v1/players/alice/balance", alice(), nil))["bonus"]
	}

	// Day 1.
	st := decode[dailyStatus](t, e.do("GET", "/v1/players/alice/daily-bonus", alice(), nil))
	if st.Claimed || st.Day != 1 || st.Reward != 500 {
		t.Fatalf("fresh status wrong: %+v", st)
	}
	if c := claim(); !c.Claimed || c.Replayed || c.Reward != 500 {
		t.Fatalf("first claim wrong: %+v", c)
	}
	// Same-day double claim must not pay twice.
	if c := claim(); !c.Replayed {
		t.Fatalf("second same-day claim should be flagged replayed: %+v", c)
	}
	if bonus() != 500 {
		t.Fatalf("double-paid: bonus=%d", bonus())
	}

	// Consecutive days climb the ladder.
	*e.clock = e.clock.AddDate(0, 0, 1)
	if c := claim(); c.Day != 2 || c.Reward != 750 {
		t.Fatalf("day 2 wrong: %+v", c)
	}
	*e.clock = e.clock.AddDate(0, 0, 1)
	if c := claim(); c.Day != 3 || c.Reward != 1000 {
		t.Fatalf("day 3 wrong: %+v", c)
	}

	// Skipping a day resets to day 1.
	*e.clock = e.clock.AddDate(0, 0, 2)
	if c := claim(); c.Day != 1 || c.Reward != 500 {
		t.Fatalf("streak should reset after a missed day: %+v", c)
	}
	if bonus() != 500+750+1000+500 {
		t.Fatalf("unexpected total bonus %d", bonus())
	}
}

func TestDailyBonusWrapsAfterDaySeven(t *testing.T) {
	e := newEnv(t)
	for day := 1; day <= 8; day++ {
		c := decode[dailyStatus](t, e.do("POST", "/v1/players/alice/daily-bonus", alice(), nil))
		want := ((day - 1) % 7) + 1
		if c.Day != want {
			t.Fatalf("day %d: ladder step %d, want %d", day, c.Day, want)
		}
		*e.clock = e.clock.AddDate(0, 0, 1)
	}
}

func TestRateLimiting(t *testing.T) {
	e := newEnv(t)
	e.srv.RateLimiter = NewRateLimiter(3, time.Minute)
	e.srv.Mux = http.NewServeMux()
	e.srv.routes("")
	var last int
	for i := 0; i < 5; i++ {
		last = e.do("GET", "/v1/players/alice/balance", alice(), nil).Code
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("want 429 got %d", last)
	}
}

func TestStaticServesGzipAndFallsBack(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>plain</html>"), 0o644)
	var gz bytes.Buffer
	zw := gzip.NewWriter(&gz)
	zw.Write([]byte("WASMDATA"))
	zw.Close()
	os.WriteFile(filepath.Join(dir, "game.wasm"), []byte("WASMDATA"), 0o644)
	os.WriteFile(filepath.Join(dir, "game.wasm.gz"), gz.Bytes(), 0o644)

	h := StaticHandler(dir)

	req := httptest.NewRequest("GET", "/game.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "gzip" || w.Header().Get("Content-Type") != "application/wasm" {
		t.Fatalf("expected gzip wasm, got headers %v", w.Header())
	}

	req = httptest.NewRequest("GET", "/game.wasm", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Header().Get("Content-Encoding") != "" || w.Body.String() != "WASMDATA" {
		t.Fatalf("expected uncompressed fallback, got %q / %v", w.Body.String(), w.Header())
	}

	req = httptest.NewRequest("GET", "/", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), "plain") {
		t.Fatalf("root should serve index.html, got %q", w.Body.String())
	}
}

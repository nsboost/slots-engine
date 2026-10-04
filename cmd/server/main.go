// cmd/server runs the HTTP API locally. Storage backend is selected by
// the LEDGER_DSN environment variable: unset or empty uses the in-memory
// store (data lost on restart — fine for local dev and demos); set it to
// a Postgres DSN to run durably. This is the ONE place backend choice is
// decided, so swapping it is a config change, not a code change.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"slots-engine/internal/api"
	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

func main() {
	eng, err := engine.New(engine.DemoFortuneReels())
	if err != nil {
		log.Fatalf("engine init failed: %v", err)
	}

	var store ledger.Store
	if dsn := os.Getenv("LEDGER_DSN"); dsn != "" {
		pg, err := ledger.OpenPGStore(dsn)
		if err != nil {
			log.Fatalf("postgres ledger init failed: %v", err)
		}
		defer pg.Close()
		store = pg
		log.Println("ledger: using Postgres backend")
	} else {
		store = ledger.NewMemStore()
		log.Println("ledger: using in-memory backend (NOT durable — set LEDGER_DSN for persistence)")
	}

	auth := api.NewStaticKeyAuthenticator(loadPlayerKeys())
	rl := api.NewRateLimiter(60, time.Minute) // 60 requests/minute/player — generous for a slot game's click rate, tight enough to blunt a naive bot

	srv := api.NewServer(eng, store, auth, rl)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("listening on %s (game version %s)", addr, engine.GameVersion)
	if err := http.ListenAndServe(addr, api.LoggingMiddleware(srv.Mux)); err != nil {
		log.Fatal(err)
	}
}

// loadPlayerKeys reads PLAYER_KEYS_JSON, a JSON object mapping playerID ->
// API key, e.g. PLAYER_KEYS_JSON='{"alice":"dev-key-alice"}'. This is the
// DEV-ONLY static-key auth store — see the doc comment on
// StaticKeyAuthenticator for what MUST replace it before this is a real
// deployment (session tokens from Apple/Google sign-in, issued and
// verified server-side, not a shared secret set via env var).
func loadPlayerKeys() map[string]string {
	raw := os.Getenv("PLAYER_KEYS_JSON")
	if raw == "" {
		log.Println("auth: PLAYER_KEYS_JSON not set — no players will be able to authenticate")
		return map[string]string{}
	}
	var keys map[string]string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		log.Fatalf("auth: failed to parse PLAYER_KEYS_JSON: %v", err)
	}
	return keys
}

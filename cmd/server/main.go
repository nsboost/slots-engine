// cmd/server runs the HTTP API (and optionally the web client).
//
// Configuration (environment variables):
//
//	LISTEN_ADDR   listen address (default :8080)
//	LEDGER_DSN    Postgres DSN; unset = in-memory ledger (NOT durable)
//	AUTH_SECRET   HMAC secret for guest tokens. Set a long random value in
//	              any real deployment; if unset a random one is generated
//	              and tokens stop working on restart.
//	STATIC_DIR    directory containing the web client build to serve at /
//	TRUST_PROXY   "true" to trust X-Forwarded-For (only behind your own proxy)
//	PLAYER_KEYS_JSON  optional dev-only static keys: {"playerId":"key"}
package main

import (
	"crypto/rand"
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
	var engines []*engine.Engine
	for _, cfg := range engine.AllGames() {
		e, err := engine.New(cfg)
		if err != nil {
			log.Fatalf("engine init failed for %s: %v", cfg.ID, err)
		}
		engines = append(engines, e)
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

	secret := []byte(os.Getenv("AUTH_SECRET"))
	if len(secret) == 0 {
		secret = make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Fatalf("failed to generate auth secret: %v", err)
		}
		log.Println("auth: AUTH_SECRET not set — generated a random one; guest tokens will NOT survive a restart")
	} else if len(secret) < 16 {
		log.Fatal("auth: AUTH_SECRET must be at least 16 bytes")
	}
	tokens := api.NewTokenAuthenticator(secret)

	authn := api.ChainAuthenticator{tokens}
	if keys := loadPlayerKeys(); len(keys) > 0 {
		authn = append(authn, api.NewStaticKeyAuthenticator(keys))
		log.Printf("auth: %d dev static key(s) enabled", len(keys))
	}

	api.SetTrustProxy(os.Getenv("TRUST_PROXY") == "true")

	srv, err := api.NewServer(api.Config{
		Engines:      engines,
		Store:        store,
		Auth:         authn,
		Tokens:       tokens,
		RateLimiter:  api.NewRateLimiter(240, time.Minute),
		GuestLimiter: api.NewRateLimiter(30, time.Hour),
		StaticDir:    os.Getenv("STATIC_DIR"),
	})
	if err != nil {
		log.Fatal(err)
	}

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	if d := os.Getenv("STATIC_DIR"); d != "" {
		log.Printf("serving web client from %s", d)
	}
	log.Printf("listening on %s (platform version %s, %d games)", addr, engine.GameVersion, len(engines))

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           api.SecurityHeaders(api.LoggingMiddleware(srv.Mux)),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func loadPlayerKeys() map[string]string {
	raw := os.Getenv("PLAYER_KEYS_JSON")
	if raw == "" {
		return nil
	}
	var keys map[string]string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		log.Fatalf("auth: failed to parse PLAYER_KEYS_JSON: %v", err)
	}
	return keys
}

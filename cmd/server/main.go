// cmd/server runs the HTTP API locally. Storage backend is selected by
// the LEDGER_DSN environment variable: unset or empty uses the in-memory
// store (data lost on restart — fine for local dev and demos); set it to
// a Postgres DSN to run durably. This is the ONE place backend choice is
// decided, so swapping it is a config change, not a code change.
package main

import (
	"log"
	"net/http"
	"os"

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

	srv := api.NewServer(eng, store)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	log.Printf("listening on %s (game version %s)", addr, engine.GameVersion)
	if err := http.ListenAndServe(addr, api.LoggingMiddleware(srv.Mux)); err != nil {
		log.Fatal(err)
	}
}

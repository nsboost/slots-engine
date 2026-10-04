// Package api exposes the game engine and ledger over HTTP for a mobile
// or web client. Handlers are deliberately thin: validate input, call into
// engine/ledger, serialize. All game logic and money logic lives in
// internal/engine and internal/ledger so it stays unit-testable without
// HTTP.
//
// File layout:
//
//	server.go            Server, Config, routing, shared helpers
//	handlers_spin.go     POST spin (idempotent, server-authoritative)
//	handlers_catalog.go  games list, store packages, demo purchase
//	handlers_account.go  guest registration, balance, history
//	handlers_daily.go    daily bonus status/claim
//	auth.go, token_auth.go, ratelimit.go, static.go  cross-cutting concerns
package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"slots-engine/internal/engine"
	"slots-engine/internal/ledger"
)

// Config bundles everything NewServer needs.
type Config struct {
	Engines      []*engine.Engine // lobby order
	Store        ledger.Store
	Auth         Authenticator       // accepts requests (chain tokens + static keys)
	Tokens       *TokenAuthenticator // issues guest tokens; nil disables guest registration
	RateLimiter  *RateLimiter        // per-player limiter for protected routes
	GuestLimiter *RateLimiter        // per-IP limiter for guest registration
	StaticDir    string              // optional: serve a web client build from here at /
	Now          func() time.Time    // injectable clock (daily bonus tests)
}

// Server holds the shared dependencies for all handlers.
type Server struct {
	Engines      map[string]*engine.Engine
	GameOrder    []string
	Store        ledger.Store
	Auth         Authenticator
	Tokens       *TokenAuthenticator
	RateLimiter  *RateLimiter
	GuestLimiter *RateLimiter
	Now          func() time.Time
	Mux          *http.ServeMux
}

func NewServer(cfg Config) (*Server, error) {
	if len(cfg.Engines) == 0 {
		return nil, errors.New("api: at least one game engine is required")
	}
	s := &Server{
		Engines:      make(map[string]*engine.Engine),
		Store:        cfg.Store,
		Auth:         cfg.Auth,
		Tokens:       cfg.Tokens,
		RateLimiter:  cfg.RateLimiter,
		GuestLimiter: cfg.GuestLimiter,
		Now:          cfg.Now,
		Mux:          http.NewServeMux(),
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	if s.RateLimiter == nil {
		s.RateLimiter = NewRateLimiter(120, time.Minute)
	}
	if s.GuestLimiter == nil {
		s.GuestLimiter = NewRateLimiter(20, time.Hour)
	}
	for _, e := range cfg.Engines {
		id := e.Config().ID
		s.Engines[id] = e
		s.GameOrder = append(s.GameOrder, id)
	}
	s.routes(cfg.StaticDir)
	return s, nil
}

func (s *Server) routes(staticDir string) {
	// Go 1.22's ServeMux does method + path-pattern routing natively, so
	// no router dependency is needed.
	//
	// Player-scoped routes: RequireAuth first (401 for bad credentials,
	// 403 for a valid credential used on another player's path), THEN the
	// rate limiter so limiting is keyed by authenticated player.
	protect := func(h http.HandlerFunc) http.Handler {
		return RequireAuth(s.Auth, s.RateLimiter.Middleware(h))
	}

	s.Mux.HandleFunc("GET /v1/healthz", s.handleHealthz)
	s.Mux.HandleFunc("GET /v1/games", s.handleGames)
	s.Mux.HandleFunc("GET /v1/store/packages", s.handlePackages)
	s.Mux.Handle("POST /v1/auth/guest", s.GuestLimiter.Middleware(http.HandlerFunc(s.handleGuest)))

	s.Mux.Handle("POST /v1/players/{playerID}/spin", protect(s.handleSpin))
	s.Mux.Handle("GET /v1/players/{playerID}/balance", protect(s.handleBalance))
	s.Mux.Handle("GET /v1/players/{playerID}/history", protect(s.handleHistory))
	s.Mux.Handle("POST /v1/players/{playerID}/demo-purchase", protect(s.handleDemoPurchase))
	s.Mux.Handle("GET /v1/players/{playerID}/daily-bonus", protect(s.handleDailyStatus))
	s.Mux.Handle("POST /v1/players/{playerID}/daily-bonus", protect(s.handleDailyClaim))

	if staticDir != "" {
		s.Mux.Handle("GET /", StaticHandler(staticDir))
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "gameVersion": engine.GameVersion})
}

// decodeJSON reads a size-limited JSON body into v.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	return json.NewDecoder(r.Body).Decode(v)
}

func randomHex(nBytes int) (string, error) {
	b := make([]byte, nBytes)
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

// LoggingMiddleware is a minimal request logger. Replace with structured
// logging and request IDs before production.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// SecurityHeaders adds baseline hardening headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

package api

import (
	"context"
	"crypto/subtle"
	"net/http"
)

// Authenticator verifies a request and returns the authenticated player's
// ID. Swap implementations (dev API keys now; real session tokens /
// Apple-Sign-In / Google-Sign-In verification later) without touching any
// handler — handlers only ever read the player ID back out of the
// request context via PlayerIDFromContext.
type Authenticator interface {
	Authenticate(r *http.Request) (playerID string, ok bool)
}

type contextKey string

const playerIDContextKey contextKey = "playerID"

// PlayerIDFromContext returns the authenticated player ID set by the auth
// middleware. Only valid on requests that passed through RequireAuth.
func PlayerIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(playerIDContextKey).(string)
	return id, ok
}

// RequireAuth wraps a handler so it 401s any request the Authenticator
// rejects, and otherwise injects the authenticated player ID into the
// request context. It also enforces that a player can only ever act on
// their own {playerID} path segment — without this check, a valid key for
// player A could be used to spend player B's coins just by changing the
// URL.
func RequireAuth(auth Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		playerID, ok := auth.Authenticate(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}

		if pathPlayer := r.PathValue("playerID"); pathPlayer != "" && pathPlayer != playerID {
			writeError(w, http.StatusForbidden, "cannot act on another player's account")
			return
		}

		ctx := context.WithValue(r.Context(), playerIDContextKey, playerID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// StaticKeyAuthenticator is a DEV-ONLY authenticator: each player has a
// single static API key, checked in constant time, sent as the
// "X-Player-Key" header. There is no key rotation, no expiry, no hashing
// at rest (keys are kept in memory as given) — this exists so the API is
// not wide open while phase 3 (the real client auth: Apple/Google sign-in
// or session tokens issued after login) is built. DO NOT deploy this
// beyond local dev / closed demo testing.
//
// Authenticate identifies the player from the KEY itself, never from the
// URL path. This is what lets RequireAuth tell apart two distinct failure
// modes correctly: an unrecognized/missing key is 401 Unauthorized, while
// a valid key for a *different* player than the URL names is 403
// Forbidden. Conflating the two (e.g. by looking up "does this key match
// the key on file for the path's playerID") would report a valid key
// used against someone else's account as "unauthorized", which is both
// the wrong HTTP semantics and a worse signal for monitoring — a 403 is a
// potential account-enumeration/attack attempt; a 401 is just a bad or
// absent credential.
type StaticKeyAuthenticator struct {
	// byKey maps API key -> owning playerID (the inverse of how keys are
	// naturally configured, built once at construction).
	byKey map[string]string
}

func NewStaticKeyAuthenticator(keys map[string]string) *StaticKeyAuthenticator {
	byKey := make(map[string]string, len(keys))
	for playerID, key := range keys {
		byKey[key] = playerID
	}
	return &StaticKeyAuthenticator{byKey: byKey}
}

func (a *StaticKeyAuthenticator) Authenticate(r *http.Request) (string, bool) {
	got := r.Header.Get("X-Player-Key")
	if got == "" {
		return "", false
	}
	// Constant-time comparison against every known key: a naive map
	// lookup on the raw header value is already effectively
	// constant-time w.r.t. any SINGLE key (Go map lookups don't leak
	// per-byte timing the way a byte-by-byte == comparison would), but we
	// compare explicitly here anyway so this stays correct even if the
	// lookup strategy changes later.
	for key, playerID := range a.byKey {
		if subtle.ConstantTimeCompare([]byte(got), []byte(key)) == 1 {
			return playerID, true
		}
	}
	return "", false
}

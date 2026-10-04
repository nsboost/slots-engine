package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// TokenAuthenticator verifies stateless signed bearer tokens of the form
//
//	v1.<playerID>.<hex(HMAC-SHA256(secret, "v1."+playerID))>
//
// issued at guest registration. Stateless means no token table to
// maintain and tokens survive server restarts as long as the secret is
// stable (AUTH_SECRET). Tradeoff, called out on purpose: a stateless token
// cannot be individually revoked — rotating the secret revokes everyone.
// When real accounts (Apple/Google sign-in) arrive, put a session store
// behind the Authenticator interface instead; nothing else changes.
type TokenAuthenticator struct {
	secret []byte
}

func NewTokenAuthenticator(secret []byte) *TokenAuthenticator {
	return &TokenAuthenticator{secret: secret}
}

func (t *TokenAuthenticator) sign(playerID string) string {
	m := hmac.New(sha256.New, t.secret)
	m.Write([]byte("v1." + playerID))
	return hex.EncodeToString(m.Sum(nil))
}

// Issue returns a bearer token for playerID.
func (t *TokenAuthenticator) Issue(playerID string) string {
	return "v1." + playerID + "." + t.sign(playerID)
}

func (t *TokenAuthenticator) Authenticate(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(h, prefix), ".")
	if len(parts) != 3 || parts[0] != "v1" || parts[1] == "" {
		return "", false
	}
	want := t.sign(parts[1])
	if !hmac.Equal([]byte(want), []byte(parts[2])) {
		return "", false
	}
	return parts[1], true
}

// ChainAuthenticator tries each authenticator in order and accepts the
// first that recognizes the request.
type ChainAuthenticator []Authenticator

func (c ChainAuthenticator) Authenticate(r *http.Request) (string, bool) {
	for _, a := range c {
		if a == nil {
			continue
		}
		if id, ok := a.Authenticate(r); ok {
			return id, true
		}
	}
	return "", false
}

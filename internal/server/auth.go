package server

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"io"
	"net/http"

	"github.com/anthropic/angelcord/internal/crypto"
	"github.com/anthropic/angelcord/internal/protocol"
)

type ctxKey string

const ctxPubKey ctxKey = "pub_key"

// AuthMiddleware supports two auth methods:
//  1. Ed25519 signed requests (CLI) via X-PubKey + X-Signature headers
//  2. JWT session auth (web app) via session cookie or Authorization header
//
// On success, stores the verified Ed25519 public key in the request context.
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try Ed25519 auth first (CLI clients)
		if pubKeyB64 := r.Header.Get("X-PubKey"); pubKeyB64 != "" {
			handleEd25519Auth(w, r, next, pubKeyB64)
			return
		}

		// Try JWT auth (web clients)
		claims, ok := getJWTClaims(r)
		if ok {
			discordID, _ := claims["discord_id"].(string)
			isNew, _ := claims["is_new_user"].(bool)
			if isNew {
				writeJSON(w, http.StatusForbidden, protocol.ErrorResponse{Error: "complete signup first"})
				return
			}
			// Look up the user's Ed25519 public key from their discord ID.
			// We need access to the DB which we get through the request context
			// set by the server setup. For now, the JWT carries the pubkey
			// info through the claims -- but we need a way to resolve discord_id -> pubkey.
			// We'll store the pubkey in the JWT claims at token creation time.
			if pubB64, ok := claims["pub_key"].(string); ok && pubB64 != "" {
				pubKey, err := crypto.PubKeyFromBase64(pubB64)
				if err == nil {
					ctx := context.WithValue(r.Context(), ctxPubKey, pubKey)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			// If no pubkey in claims, try to look it up via discord_id
			// We'll set a special context for this
			_ = discordID
			writeJSON(w, http.StatusUnauthorized, protocol.ErrorResponse{Error: "session missing pub_key"})
			return
		}

		writeJSON(w, http.StatusUnauthorized, protocol.ErrorResponse{Error: "authentication required"})
	})
}

func handleEd25519Auth(w http.ResponseWriter, r *http.Request, next http.Handler, pubKeyB64 string) {
	pubKey, err := crypto.PubKeyFromBase64(pubKeyB64)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, protocol.ErrorResponse{Error: "invalid public key"})
		return
	}

	if r.Method != http.MethodGet {
		sigB64 := r.Header.Get("X-Signature")
		if sigB64 == "" {
			writeJSON(w, http.StatusUnauthorized, protocol.ErrorResponse{Error: "missing X-Signature header"})
			return
		}
		sig, err := base64.StdEncoding.DecodeString(sigB64)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, protocol.ErrorResponse{Error: "invalid signature encoding"})
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, protocol.ErrorResponse{Error: "cannot read body"})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		path := r.URL.RawPath
		if path == "" {
			path = r.URL.Path
		}
		if !crypto.VerifyRequestSignature(pubKey, r.Method, path, body, sig) {
			writeJSON(w, http.StatusForbidden, protocol.ErrorResponse{Error: "invalid signature"})
			return
		}
	}

	ctx := context.WithValue(r.Context(), ctxPubKey, pubKey)
	next.ServeHTTP(w, r.WithContext(ctx))
}

func getPubKey(r *http.Request) ed25519.PublicKey {
	return r.Context().Value(ctxPubKey).(ed25519.PublicKey)
}

func getPubKeyB64(r *http.Request) string {
	return base64.StdEncoding.EncodeToString(getPubKey(r))
}

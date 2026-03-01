package crypto

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/curve25519"
)

// Identity holds an Ed25519 keypair and its derived X25519 keypair.
type Identity struct {
	EdPriv ed25519.PrivateKey
	EdPub  ed25519.PublicKey
	XPriv  [32]byte // X25519 private key (derived from Ed25519)
	XPub   [32]byte // X25519 public key  (derived from Ed25519)
}

// GenerateIdentity creates a new Ed25519 keypair and derives the
// corresponding X25519 keypair for key agreement.
func GenerateIdentity() (*Identity, error) {
	edPub, edPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	xPriv := edPrivateToX25519(edPriv)
	xPub, err := EdPublicToX25519(edPub)
	if err != nil {
		return nil, err
	}
	return &Identity{EdPriv: edPriv, EdPub: edPub, XPriv: xPriv, XPub: xPub}, nil
}

// PubKeyBase64 returns the base64-encoded Ed25519 public key.
func (id *Identity) PubKeyBase64() string {
	return base64.StdEncoding.EncodeToString(id.EdPub)
}

// Sign signs a message with the Ed25519 private key and returns the
// raw 64-byte signature.
func (id *Identity) Sign(message []byte) []byte {
	return ed25519.Sign(id.EdPriv, message)
}

// VerifySignature checks an Ed25519 signature against a public key.
func VerifySignature(pubKey ed25519.PublicKey, message, sig []byte) bool {
	return ed25519.Verify(pubKey, message, sig)
}

// SignRequest produces the signature material for authenticated API requests.
// The signed payload is: method + path + body (concatenated).
func (id *Identity) SignRequest(method, path string, body []byte) []byte {
	msg := buildRequestSigningPayload(method, path, body)
	return id.Sign(msg)
}

// VerifyRequestSignature verifies an API request signature.
func VerifyRequestSignature(pubKey ed25519.PublicKey, method, path string, body, sig []byte) bool {
	msg := buildRequestSigningPayload(method, path, body)
	return ed25519.Verify(pubKey, msg, sig)
}

func buildRequestSigningPayload(method, path string, body []byte) []byte {
	// method + "\n" + path + "\n" + body
	payload := make([]byte, 0, len(method)+1+len(path)+1+len(body))
	payload = append(payload, method...)
	payload = append(payload, '\n')
	payload = append(payload, path...)
	payload = append(payload, '\n')
	payload = append(payload, body...)
	return payload
}

// ECDH performs X25519 Diffie-Hellman key agreement between our private
// key and the peer's public key, returning the 32-byte shared secret.
func (id *Identity) ECDH(peerXPub [32]byte) ([]byte, error) {
	return curve25519.X25519(id.XPriv[:], peerXPub[:])
}

// PubKeyFromBase64 decodes a base64-encoded Ed25519 public key.
func PubKeyFromBase64(s string) (ed25519.PublicKey, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid key length: got %d, want %d", len(b), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(b), nil
}

// edPrivateToX25519 derives an X25519 private key from an Ed25519 private key.
// This follows the standard algorithm used by libsodium and NaCl.
func edPrivateToX25519(edPriv ed25519.PrivateKey) [32]byte {
	h := sha512.Sum512(edPriv.Seed())
	var xPriv [32]byte
	copy(xPriv[:], h[:32])
	// Clamp per RFC 7748
	xPriv[0] &= 248
	xPriv[31] &= 127
	xPriv[31] |= 64
	return xPriv
}

// EdPublicToX25519 converts an Ed25519 public key to X25519 using the
// birational equivalence between Edwards25519 and Curve25519.
func EdPublicToX25519(edPub ed25519.PublicKey) ([32]byte, error) {
	p, err := new(edwards25519.Point).SetBytes(edPub)
	if err != nil {
		return [32]byte{}, fmt.Errorf("invalid Ed25519 public key: %w", err)
	}
	xPub := p.BytesMontgomery()
	var out [32]byte
	copy(out[:], xPub)
	return out, nil
}

// MessageSigningPayload builds the payload that gets signed inside encrypted
// messages: sender_pub + channel_id + content + seq (for tamper detection).
// Including sender_pub prevents a rogue server from reassigning messages to a
// different sender — the sig now covers sender identity.
func MessageSigningPayload(senderPub, channelID, content string, seq uint64) []byte {
	payload := fmt.Sprintf("%s\n%s\n%s\n%d", senderPub, channelID, content, seq)
	return []byte(payload)
}

// SignMessage signs the inner message fields with the Ed25519 private key.
func (id *Identity) SignMessage(channelID, content string, seq uint64) []byte {
	payload := MessageSigningPayload(id.PubKeyBase64(), channelID, content, seq)
	sig, _ := id.EdPriv.Sign(rand.Reader, payload, crypto.Hash(0))
	return sig
}

package crypto

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
)

const GuildKeySize = 32

// GenerateGuildKey creates a random 32-byte symmetric key for a guild.
func GenerateGuildKey() ([]byte, error) {
	key := make([]byte, GuildKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// EncryptMessage encrypts plaintext with a guild key using XChaCha20-Poly1305.
// Returns nonce || ciphertext.
func EncryptMessage(key, plaintext []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create AEAD: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// DecryptMessage decrypts a message encrypted with EncryptMessage.
// Input is nonce || ciphertext.
func DecryptMessage(key, sealed []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, fmt.Errorf("create AEAD: %w", err)
	}
	if len(sealed) < aead.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := sealed[:aead.NonceSize()]
	ciphertext := sealed[aead.NonceSize():]
	return aead.Open(nil, nonce, ciphertext, nil)
}

// SealKey encrypts a 32-byte key so only the holder of recipientXPriv can
// decrypt it, using X25519 ECDH + XChaCha20-Poly1305.
func SealKey(key []byte, senderXPriv [32]byte, recipientXPub [32]byte) ([]byte, error) {
	shared, err := curve25519.X25519(senderXPriv[:], recipientXPub[:])
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}
	symKey := sha256.Sum256(shared)
	return EncryptMessage(symKey[:], key)
}

// UnsealKey decrypts a sealed key using X25519 ECDH.
func UnsealKey(sealed []byte, recipientXPriv [32]byte, senderXPub [32]byte) ([]byte, error) {
	shared, err := curve25519.X25519(recipientXPriv[:], senderXPub[:])
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}
	symKey := sha256.Sum256(shared)
	return DecryptMessage(symKey[:], sealed)
}

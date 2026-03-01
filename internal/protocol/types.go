// Package protocol defines the shared wire types used by both the angelcord
// server and client. These are the JSON structures that flow over the REST API.
package protocol

import "time"

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

type RegisterRequest struct {
	Name   string `json:"name"`
	PubKey string `json:"pub_key"` // base64 Ed25519 public key
}

type User struct {
	Name   string `json:"name"`
	PubKey string `json:"pub_key"`
}

// ---------------------------------------------------------------------------
// Guilds
// ---------------------------------------------------------------------------

type CreateGuildRequest struct {
	Name string `json:"name"`
}

type GuildResponse struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	OwnerPub string   `json:"owner_pub"`
	Members  []string `json:"members"` // pubkeys
}

// ---------------------------------------------------------------------------
// Members / Invite
// ---------------------------------------------------------------------------

type InviteRequest struct {
	InviteePubKey string           `json:"invitee_pub_key"`
	SealedKeys    []SealedKeyEntry `json:"sealed_keys"` // guild key sealed for invitee
}

// ---------------------------------------------------------------------------
// Sealed Key Bundles (guild key sealed per-member)
// ---------------------------------------------------------------------------

type SealedKeyEntry struct {
	SealedKeyB64 string `json:"sealed_key_b64"`
	SealerPub    string `json:"sealer_pub"`
}

type KeyBundle struct {
	GuildID      string           `json:"guild_id"`
	RecipientPub string           `json:"recipient_pub"`
	Keys         []SealedKeyEntry `json:"keys"`
}

type UploadKeysRequest struct {
	Bundles []KeyBundle `json:"bundles"` // one per recipient
}

// ---------------------------------------------------------------------------
// Messages (server-side record — ciphertext only)
// ---------------------------------------------------------------------------

type SendMessageRequest struct {
	CiphertextB64 string `json:"ciphertext_b64"`
}

type MessageEnvelope struct {
	ID            string    `json:"id"`
	GuildID       string    `json:"guild_id"`
	CiphertextB64 string    `json:"ciphertext_b64"`
	Timestamp     time.Time `json:"timestamp"`
}

// MessageInner is the JSON structure that the client encrypts before sending.
// The server never sees this — it only exists inside the ciphertext blob.
type MessageInner struct {
	SenderPub string `json:"sender_pub"` // Ed25519 pubkey of sender
	ChannelID string `json:"channel_id"`
	Content   string `json:"content"`
	Seq       uint64 `json:"seq"`
	Sig       string `json:"sig"` // base64 Ed25519 signature of (sender_pub + channel_id + content + seq)
}

// ---------------------------------------------------------------------------
// DM messages (Signal Protocol encrypted blobs)
// ---------------------------------------------------------------------------

type DMMessage struct {
	ID            string    `json:"id"`
	SenderPub     string    `json:"sender_pub"`
	RecipientPub  string    `json:"recipient_pub"`
	CiphertextB64 string    `json:"ciphertext_b64"`
	MessageType   int       `json:"message_type"`
	Timestamp     time.Time `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// Generic error response
// ---------------------------------------------------------------------------

type ErrorResponse struct {
	Error string `json:"error"`
}

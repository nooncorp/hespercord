package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropic/angelcord/internal/crypto"
	"github.com/anthropic/angelcord/internal/protocol"
)

// Session is the client-side orchestrator. It holds a single user's Ed25519
// identity, guild keys, and an HTTP client. All encryption and decryption
// happens here -- the server never sees plaintext.
type Session struct {
	Identity *crypto.Identity
	HTTP     *HTTPClient
	User     *protocol.User

	// guildKeys[guildID] = 32-byte symmetric guild key
	guildKeys map[string][]byte

	// seqCounters[guildID] = next sequence number for sending
	seqCounters map[string]uint64

	// peerSeq[guildID][senderPub] = last seen seq from that sender
	peerSeq map[string]map[string]uint64

	// decryptedCache[messageID] = already-decrypted result (avoids re-processing)
	decryptedCache map[string]*DecryptedMessage
}

// NewSession generates a fresh Ed25519 identity, registers with the server,
// and returns a ready-to-use Session.
func NewSession(name, serverURL string) (*Session, error) {
	id, err := crypto.GenerateIdentity()
	if err != nil {
		return nil, fmt.Errorf("generate identity: %w", err)
	}

	tempClient := NewHTTPClient(serverURL, nil)
	user, err := tempClient.Register(name, id.PubKeyBase64())
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}

	httpClient := NewHTTPClient(serverURL, id)

	return &Session{
		Identity:       id,
		HTTP:           httpClient,
		User:           user,
		guildKeys:      make(map[string][]byte),
		seqCounters:    make(map[string]uint64),
		peerSeq:        make(map[string]map[string]uint64),
		decryptedCache: make(map[string]*DecryptedMessage),
	}, nil
}

// PubKeyB64 returns this session's base64-encoded Ed25519 public key.
func (s *Session) PubKeyB64() string {
	return s.Identity.PubKeyBase64()
}

// ---------------------------------------------------------------------------
// Guild creation + key setup
// ---------------------------------------------------------------------------

// CreateGuild creates a guild and generates a guild key. Uploads the sealed
// key for ourselves.
func (s *Session) CreateGuild(name string) (*protocol.GuildResponse, error) {
	g, err := s.HTTP.CreateGuild(name)
	if err != nil {
		return nil, err
	}

	guildKey, err := crypto.GenerateGuildKey()
	if err != nil {
		return nil, fmt.Errorf("generate guild key: %w", err)
	}

	s.guildKeys[g.ID] = guildKey

	sealed, err := crypto.SealKey(guildKey, s.Identity.XPriv, s.Identity.XPub)
	if err != nil {
		return nil, fmt.Errorf("seal guild key for self: %w", err)
	}

	err = s.HTTP.UploadKeys(g.ID, protocol.UploadKeysRequest{
		Bundles: []protocol.KeyBundle{{
			GuildID:      g.ID,
			RecipientPub: s.PubKeyB64(),
			Keys: []protocol.SealedKeyEntry{{
				SealedKeyB64: base64.StdEncoding.EncodeToString(sealed),
				SealerPub:    s.PubKeyB64(),
			}},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("upload guild key: %w", err)
	}

	return g, nil
}

// InviteMember invites a member and seals the guild key for them.
func (s *Session) InviteMember(guildID, inviteePubB64 string) error {
	inviteeEdPub, err := crypto.PubKeyFromBase64(inviteePubB64)
	if err != nil {
		return fmt.Errorf("invalid invitee public key: %w", err)
	}
	inviteeXPub, err := crypto.EdPublicToX25519(inviteeEdPub)
	if err != nil {
		return fmt.Errorf("derive X25519 key: %w", err)
	}

	guildKey, ok := s.guildKeys[guildID]
	if !ok {
		return fmt.Errorf("no guild key for guild %s", guildID)
	}

	sealed, err := crypto.SealKey(guildKey, s.Identity.XPriv, inviteeXPub)
	if err != nil {
		return fmt.Errorf("seal guild key: %w", err)
	}

	return s.HTTP.Invite(guildID, protocol.InviteRequest{
		InviteePubKey: inviteePubB64,
		SealedKeys: []protocol.SealedKeyEntry{{
			SealedKeyB64: base64.StdEncoding.EncodeToString(sealed),
			SealerPub:    s.PubKeyB64(),
		}},
	})
}

// KickMember removes a member. The server enforces that kicked members can
// no longer read or write messages.
func (s *Session) KickMember(guildID, targetPubB64 string) error {
	return s.HTTP.Kick(guildID, targetPubB64)
}

// SyncGuildKey downloads and decrypts the guild key from the server.
// Only runs if we don't already have the key locally.
func (s *Session) SyncGuildKey(guildID string) error {
	if _, ok := s.guildKeys[guildID]; ok {
		return nil
	}

	bundle, err := s.HTTP.GetKeys(guildID)
	if err != nil {
		return fmt.Errorf("get keys: %w", err)
	}

	if len(bundle.Keys) == 0 {
		return fmt.Errorf("no sealed keys available for guild %s", guildID)
	}

	entry := bundle.Keys[0]
	sealerEdPub, err := crypto.PubKeyFromBase64(entry.SealerPub)
	if err != nil {
		return fmt.Errorf("invalid sealer pub key: %w", err)
	}
	sealerXPub, err := crypto.EdPublicToX25519(sealerEdPub)
	if err != nil {
		return fmt.Errorf("derive sealer X25519 key: %w", err)
	}

	sealedBytes, err := base64.StdEncoding.DecodeString(entry.SealedKeyB64)
	if err != nil {
		return fmt.Errorf("decode sealed key: %w", err)
	}

	guildKey, err := crypto.UnsealKey(sealedBytes, s.Identity.XPriv, sealerXPub)
	if err != nil {
		return fmt.Errorf("unseal guild key: %w", err)
	}

	s.guildKeys[guildID] = guildKey
	return nil
}

// ---------------------------------------------------------------------------
// Messaging
// ---------------------------------------------------------------------------

// SendMessage signs, encrypts, and sends a message.
func (s *Session) SendMessage(guildID, channelID, content string) error {
	if err := s.SyncGuildKey(guildID); err != nil {
		return fmt.Errorf("sync key: %w", err)
	}

	guildKey, ok := s.guildKeys[guildID]
	if !ok {
		return fmt.Errorf("no guild key for guild %s", guildID)
	}

	seq := s.seqCounters[guildID]
	s.seqCounters[guildID] = seq + 1

	sig := s.Identity.SignMessage(channelID, content, seq)

	inner := protocol.MessageInner{
		SenderPub: s.PubKeyB64(),
		ChannelID: channelID,
		Content:   content,
		Seq:       seq,
		Sig:       base64.StdEncoding.EncodeToString(sig),
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return err
	}

	ciphertext, err := crypto.EncryptMessage(guildKey, innerJSON)
	if err != nil {
		return fmt.Errorf("encrypt: %w", err)
	}

	_, err = s.HTTP.SendMessage(guildID, protocol.SendMessageRequest{
		CiphertextB64: base64.StdEncoding.EncodeToString(ciphertext),
	})
	return err
}

// DecryptedMessage is a message after client-side decryption and verification.
type DecryptedMessage struct {
	ID        string
	SenderPub string
	ChannelID string
	Content   string
	Seq       uint64
	Timestamp time.Time
	Error     string
}

// ReadMessages fetches guild messages, decrypts them, and optionally filters
// by channel. Pass "" for channelID to return all channels.
func (s *Session) ReadMessages(guildID, channelID string) ([]DecryptedMessage, error) {
	if err := s.SyncGuildKey(guildID); err != nil {
		return nil, fmt.Errorf("sync key: %w", err)
	}

	envelopes, err := s.HTTP.GetMessages(guildID, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("fetch messages: %w", err)
	}

	var result []DecryptedMessage
	for _, env := range envelopes {
		if cached, ok := s.decryptedCache[env.ID]; ok {
			if channelID == "" || cached.ChannelID == channelID {
				result = append(result, *cached)
			}
			continue
		}
		dm := s.decryptEnvelope(env)
		s.decryptedCache[env.ID] = &dm
		if channelID == "" || dm.ChannelID == channelID {
			result = append(result, dm)
		}
	}
	return result, nil
}

func (s *Session) decryptEnvelope(env protocol.MessageEnvelope) DecryptedMessage {
	dm := DecryptedMessage{
		ID:        env.ID,
		Timestamp: env.Timestamp,
	}

	guildKey, ok := s.guildKeys[env.GuildID]
	if !ok {
		dm.Error = "missing guild key"
		return dm
	}

	ctBytes, err := base64.StdEncoding.DecodeString(env.CiphertextB64)
	if err != nil {
		dm.Error = "base64 decode: " + err.Error()
		return dm
	}

	plaintext, err := crypto.DecryptMessage(guildKey, ctBytes)
	if err != nil {
		dm.Error = "decrypt: " + err.Error()
		return dm
	}

	var inner protocol.MessageInner
	if err := json.Unmarshal(plaintext, &inner); err != nil {
		dm.Error = "parse envelope: " + err.Error()
		return dm
	}

	dm.SenderPub = inner.SenderPub

	senderEdPub, err := crypto.PubKeyFromBase64(inner.SenderPub)
	if err == nil {
		sigBytes, err := base64.StdEncoding.DecodeString(inner.Sig)
		if err != nil {
			dm.Error = "invalid sig encoding"
			return dm
		}
		payload := crypto.MessageSigningPayload(inner.SenderPub, inner.ChannelID, inner.Content, inner.Seq)
		if !crypto.VerifySignature(senderEdPub, payload, sigBytes) {
			dm.Error = "signature verification failed"
			return dm
		}
	}

	if s.peerSeq[env.GuildID] == nil {
		s.peerSeq[env.GuildID] = make(map[string]uint64)
	}
	lastSeq, seen := s.peerSeq[env.GuildID][inner.SenderPub]
	if seen && inner.Seq <= lastSeq {
		dm.Error = fmt.Sprintf("seq %d <= last seen %d (possible replay/reorder)", inner.Seq, lastSeq)
		return dm
	}
	s.peerSeq[env.GuildID][inner.SenderPub] = inner.Seq

	dm.ChannelID = inner.ChannelID
	dm.Content = inner.Content
	dm.Seq = inner.Seq
	return dm
}

package server

import (
	"fmt"
	"sync"
	"time"

	"github.com/anthropic/angelcord/internal/protocol"
	"github.com/google/uuid"
)

// Verify RelayStore satisfies Storage at compile time.
var _ Storage = (*RelayStore)(nil)

// RelayStore holds relay-specific data that stays on the server regardless
// of where guild governance state lives. This includes user registrations,
// encrypted message blobs, and sealed guild key bundles.
type RelayStore struct {
	mu            sync.RWMutex
	users         map[string]*protocol.User                       // pubkey -> user
	messages      map[string][]protocol.MessageEnvelope           // guildID -> messages
	keys          map[string]map[string][]protocol.SealedKeyEntry // guildID -> recipientPub -> sealed keys
	dms           map[string][]protocol.DMMessage                 // sorted-key -> messages
	signalBundles map[string]SignalPreKeyBundle                   // pubkey -> bundle
	signalOTKs    map[string][]SignalOneTimePreKey                // pubkey -> one-time prekeys
}

func NewRelayStore() *RelayStore {
	return &RelayStore{
		users:         make(map[string]*protocol.User),
		messages:      make(map[string][]protocol.MessageEnvelope),
		keys:          make(map[string]map[string][]protocol.SealedKeyEntry),
		dms:           make(map[string][]protocol.DMMessage),
		signalBundles: make(map[string]SignalPreKeyBundle),
		signalOTKs:    make(map[string][]SignalOneTimePreKey),
	}
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func (s *RelayStore) RegisterUser(name, pubKey string) *protocol.User {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := &protocol.User{Name: name, PubKey: pubKey}
	s.users[pubKey] = u
	return u
}

func (s *RelayStore) GetUser(pubKey string) (*protocol.User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.users[pubKey]
	return u, ok
}

func (s *RelayStore) UserExists(pubKey string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.users[pubKey]
	return ok
}

func (s *RelayStore) ListUsers() []protocol.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]protocol.User, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, *u)
	}
	return out
}

// ---------------------------------------------------------------------------
// Guild Key Bundles
// ---------------------------------------------------------------------------

func (s *RelayStore) StoreKeys(guildID, recipientPub string, keys []protocol.SealedKeyEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keys[guildID] == nil {
		s.keys[guildID] = make(map[string][]protocol.SealedKeyEntry)
	}
	s.keys[guildID][recipientPub] = keys
}

func (s *RelayStore) GetKeys(guildID, recipientPub string) []protocol.SealedKeyEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := s.keys[guildID][recipientPub]
	out := make([]protocol.SealedKeyEntry, len(keys))
	copy(out, keys)
	return out
}

// RemoveKeys removes all key bundles for a specific member in a guild
// (used during kick).
func (s *RelayStore) RemoveKeys(guildID, pubKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.keys[guildID]; m != nil {
		delete(m, pubKey)
	}
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func (s *RelayStore) StoreMessage(guildID, senderPub, ciphertextB64 string) (*protocol.MessageEnvelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	env := protocol.MessageEnvelope{
		ID:            uuid.New().String(),
		GuildID:       guildID,
		SenderPub:     senderPub,
		CiphertextB64: ciphertextB64,
		Timestamp:     time.Now(),
	}
	s.messages[guildID] = append(s.messages[guildID], env)
	return &env, nil
}

func (s *RelayStore) ListMessages(guildID string, after time.Time) []protocol.MessageEnvelope {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := s.messages[guildID]
	if after.IsZero() {
		out := make([]protocol.MessageEnvelope, len(all))
		copy(out, all)
		return out
	}
	var out []protocol.MessageEnvelope
	for _, m := range all {
		if m.Timestamp.After(after) {
			out = append(out, m)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// DMs (in-memory)
// ---------------------------------------------------------------------------

func (s *RelayStore) StoreDMMessage(senderPub, recipientPub, ciphertextB64 string, messageType int) (*protocol.DMMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if messageType == 0 {
		messageType = 2
	}
	msg := protocol.DMMessage{
		ID:            uuid.New().String(),
		SenderPub:     senderPub,
		RecipientPub:  recipientPub,
		CiphertextB64: ciphertextB64,
		MessageType:   messageType,
		Timestamp:     time.Now(),
	}
	key := dmKey(senderPub, recipientPub)
	s.dms[key] = append(s.dms[key], msg)
	return &msg, nil
}

func (s *RelayStore) ListDMMessages(pub1, pub2 string, after time.Time) []protocol.DMMessage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := dmKey(pub1, pub2)
	all := s.dms[key]
	if after.IsZero() {
		out := make([]protocol.DMMessage, len(all))
		copy(out, all)
		return out
	}
	var out []protocol.DMMessage
	for _, m := range all {
		if m.Timestamp.After(after) {
			out = append(out, m)
		}
	}
	return out
}

func (s *RelayStore) ListDMConversations(pubKey string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for k, msgs := range s.dms {
		for _, m := range msgs {
			if m.SenderPub == pubKey && !seen[m.RecipientPub] {
				seen[m.RecipientPub] = true
			} else if m.RecipientPub == pubKey && !seen[m.SenderPub] {
				seen[m.SenderPub] = true
			}
		}
		_ = k
	}
	out := make([]string, 0, len(seen))
	for peer := range seen {
		out = append(out, peer)
	}
	return out
}

func dmKey(a, b string) string {
	if a < b {
		return a + "|" + b
	}
	return b + "|" + a
}

// ---------------------------------------------------------------------------
// Signal Pre-Keys (in-memory)
// ---------------------------------------------------------------------------

func (s *RelayStore) StoreSignalPreKeyBundle(b SignalPreKeyBundle) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signalBundles[b.PubKey] = b
	return nil
}

func (s *RelayStore) StoreSignalOneTimePreKeys(pubKey string, keys []SignalOneTimePreKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.signalOTKs[pubKey] = append(s.signalOTKs[pubKey], keys...)
	return nil
}

func (s *RelayStore) GetSignalPreKeyBundle(pubKey string) (*SignalPreKeyBundle, *SignalOneTimePreKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.signalBundles[pubKey]
	if !ok {
		return nil, nil, fmt.Errorf("no bundle")
	}
	var otk *SignalOneTimePreKey
	if otks := s.signalOTKs[pubKey]; len(otks) > 0 {
		otk = &otks[0]
		s.signalOTKs[pubKey] = otks[1:]
	}
	return &b, otk, nil
}

func (s *RelayStore) SetGuildMemberAvatar(guildID, memberPub, encryptedAvatarB64 string) error {
	return nil
}

func (s *RelayStore) GetGuildMemberAvatars(guildID string) ([]GuildMemberAvatar, error) {
	return nil, nil
}

package guild

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
)

type memGuild struct {
	ID          string
	Name        string
	OwnerPubKey string
	Members     []string // ordered pubkeys
	memberSet   map[string]bool
}

// MemoryGuildState is the in-memory PoC implementation of GuildState.
type MemoryGuildState struct {
	mu     sync.RWMutex
	guilds map[string]*memGuild
}

func NewMemoryGuildState() *MemoryGuildState {
	return &MemoryGuildState{guilds: make(map[string]*memGuild)}
}

func (m *MemoryGuildState) CreateGuild(name, ownerPubKey string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := uuid.New().String()
	g := &memGuild{
		ID:          id,
		Name:        name,
		OwnerPubKey: ownerPubKey,
		Members:     []string{ownerPubKey},
		memberSet:   map[string]bool{ownerPubKey: true},
	}
	m.guilds[id] = g
	return id, nil
}

func (m *MemoryGuildState) GetGuild(id string) (*GuildInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.guilds[id]
	if !ok {
		return nil, fmt.Errorf("guild %s not found", id)
	}
	return g.toInfo(), nil
}

func (m *MemoryGuildState) ListGuildsForUser(pubKey string) ([]GuildInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []GuildInfo
	for _, g := range m.guilds {
		if g.memberSet[pubKey] {
			out = append(out, *g.toInfo())
		}
	}
	return out, nil
}

func (m *MemoryGuildState) AddMember(guildID, pubKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.guilds[guildID]
	if !ok {
		return fmt.Errorf("guild %s not found", guildID)
	}
	if g.memberSet[pubKey] {
		return fmt.Errorf("already a member")
	}
	g.memberSet[pubKey] = true
	g.Members = append(g.Members, pubKey)
	return nil
}

func (m *MemoryGuildState) RemoveMember(guildID, pubKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.guilds[guildID]
	if !ok {
		return fmt.Errorf("guild %s not found", guildID)
	}
	if !g.memberSet[pubKey] {
		return fmt.Errorf("not a member")
	}
	if g.OwnerPubKey == pubKey {
		return fmt.Errorf("cannot remove the guild owner")
	}
	delete(g.memberSet, pubKey)
	for i, pk := range g.Members {
		if pk == pubKey {
			g.Members = append(g.Members[:i], g.Members[i+1:]...)
			break
		}
	}
	return nil
}

func (m *MemoryGuildState) IsMember(guildID, pubKey string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.guilds[guildID]
	if !ok {
		return false, fmt.Errorf("guild %s not found", guildID)
	}
	return g.memberSet[pubKey], nil
}

func (m *MemoryGuildState) ListMembers(guildID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	g, ok := m.guilds[guildID]
	if !ok {
		return nil, fmt.Errorf("guild %s not found", guildID)
	}
	out := make([]string, len(g.Members))
	copy(out, g.Members)
	return out, nil
}

func (g *memGuild) toInfo() *GuildInfo {
	members := make([]string, len(g.Members))
	copy(members, g.Members)
	return &GuildInfo{
		ID:          g.ID,
		Name:        g.Name,
		OwnerPubKey: g.OwnerPubKey,
		Members:     members,
	}
}

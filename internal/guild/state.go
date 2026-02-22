// Package guild defines the GuildState interface that abstracts guild
// governance data. The PoC uses an in-memory implementation; future versions
// swap in a Solana on-chain implementation with zero changes to the relay.
package guild

// GuildInfo holds the metadata for a guild as returned by GuildState.
type GuildInfo struct {
	ID          string
	Name        string
	OwnerPubKey string
	Members     []string // Ed25519 public keys (base64)
}

// GuildState is the interface for guild governance data.
// PoC: in-memory implementation (MemoryGuildState).
// Future: backed by Solana on-chain program accounts.
type GuildState interface {
	CreateGuild(name, ownerPubKey string) (guildID string, err error)
	GetGuild(id string) (*GuildInfo, error)
	ListGuildsForUser(pubKey string) ([]GuildInfo, error)

	AddMember(guildID, pubKey string) error
	RemoveMember(guildID, pubKey string) error
	IsMember(guildID, pubKey string) (bool, error)
	ListMembers(guildID string) ([]string, error) // returns pubkeys
}

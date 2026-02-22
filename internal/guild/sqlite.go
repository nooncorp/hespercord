package guild

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// SQLiteGuildState implements the GuildState interface using SQLite.
type SQLiteGuildState struct {
	db *sql.DB
}

func NewSQLiteGuildState(db *sql.DB) *SQLiteGuildState {
	return &SQLiteGuildState{db: db}
}

func (s *SQLiteGuildState) CreateGuild(name, ownerPubKey string) (string, error) {
	id := uuid.New().String()
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO guilds (id, name, owner_pub) VALUES (?, ?, ?)`, id, name, ownerPubKey); err != nil {
		tx.Rollback()
		return "", err
	}
	if _, err := tx.Exec(`INSERT INTO guild_members (guild_id, pub_key) VALUES (?, ?)`, id, ownerPubKey); err != nil {
		tx.Rollback()
		return "", err
	}
	return id, tx.Commit()
}

func (s *SQLiteGuildState) GetGuild(id string) (*GuildInfo, error) {
	row := s.db.QueryRow(`SELECT id, name, owner_pub FROM guilds WHERE id = ?`, id)
	var g GuildInfo
	if err := row.Scan(&g.ID, &g.Name, &g.OwnerPubKey); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("guild %s not found", id)
		}
		return nil, err
	}
	members, err := s.ListMembers(id)
	if err != nil {
		return nil, err
	}
	g.Members = members
	return &g, nil
}

func (s *SQLiteGuildState) ListGuildsForUser(pubKey string) ([]GuildInfo, error) {
	rows, err := s.db.Query(
		`SELECT g.id, g.name, g.owner_pub FROM guilds g
		 JOIN guild_members gm ON g.id = gm.guild_id
		 WHERE gm.pub_key = ?`, pubKey)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuildInfo
	for rows.Next() {
		var g GuildInfo
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerPubKey); err != nil {
			return nil, err
		}
		members, _ := s.ListMembers(g.ID)
		g.Members = members
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *SQLiteGuildState) AddMember(guildID, pubKey string) error {
	var exists int
	s.db.QueryRow(`SELECT COUNT(*) FROM guilds WHERE id = ?`, guildID).Scan(&exists)
	if exists == 0 {
		return fmt.Errorf("guild %s not found", guildID)
	}
	var already int
	s.db.QueryRow(`SELECT COUNT(*) FROM guild_members WHERE guild_id = ? AND pub_key = ?`, guildID, pubKey).Scan(&already)
	if already > 0 {
		return fmt.Errorf("already a member")
	}
	_, err := s.db.Exec(`INSERT INTO guild_members (guild_id, pub_key) VALUES (?, ?)`, guildID, pubKey)
	return err
}

func (s *SQLiteGuildState) RemoveMember(guildID, pubKey string) error {
	var ownerPub string
	err := s.db.QueryRow(`SELECT owner_pub FROM guilds WHERE id = ?`, guildID).Scan(&ownerPub)
	if err != nil {
		return fmt.Errorf("guild %s not found", guildID)
	}
	if ownerPub == pubKey {
		return fmt.Errorf("cannot remove the guild owner")
	}
	var isMember int
	s.db.QueryRow(`SELECT COUNT(*) FROM guild_members WHERE guild_id = ? AND pub_key = ?`, guildID, pubKey).Scan(&isMember)
	if isMember == 0 {
		return fmt.Errorf("not a member")
	}
	_, err = s.db.Exec(`DELETE FROM guild_members WHERE guild_id = ? AND pub_key = ?`, guildID, pubKey)
	return err
}

func (s *SQLiteGuildState) IsMember(guildID, pubKey string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM guild_members WHERE guild_id = ? AND pub_key = ?`, guildID, pubKey).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *SQLiteGuildState) ListMembers(guildID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT pub_key FROM guild_members WHERE guild_id = ?`, guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var pk string
		rows.Scan(&pk)
		out = append(out, pk)
	}
	return out, rows.Err()
}

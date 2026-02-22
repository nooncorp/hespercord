package server

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/anthropic/angelcord/internal/protocol"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
	discord_id        TEXT PRIMARY KEY,
	username          TEXT NOT NULL,
	avatar_url        TEXT NOT NULL DEFAULT '',
	ed25519_pub       TEXT NOT NULL UNIQUE,
	x25519_pub        TEXT NOT NULL,
	encrypted_privkey BLOB NOT NULL,
	key_salt          BLOB NOT NULL,
	key_iterations    INTEGER NOT NULL,
	created_at        DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS guild_member_avatars (
	guild_id             TEXT NOT NULL,
	member_pub           TEXT NOT NULL,
	encrypted_avatar_b64 TEXT NOT NULL,
	PRIMARY KEY (guild_id, member_pub)
);

CREATE TABLE IF NOT EXISTS guilds (
	id        TEXT PRIMARY KEY,
	name      TEXT NOT NULL,
	owner_pub TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS guild_members (
	guild_id TEXT NOT NULL,
	pub_key  TEXT NOT NULL,
	PRIMARY KEY (guild_id, pub_key)
);

CREATE TABLE IF NOT EXISTS sealed_keys (
	guild_id      TEXT NOT NULL,
	recipient_pub TEXT NOT NULL,
	sealed_key_b64 TEXT NOT NULL,
	sealer_pub    TEXT NOT NULL,
	PRIMARY KEY (guild_id, recipient_pub)
);

CREATE TABLE IF NOT EXISTS messages (
	id             TEXT PRIMARY KEY,
	guild_id       TEXT NOT NULL,
	sender_pub     TEXT NOT NULL,
	ciphertext_b64 TEXT NOT NULL,
	created_at     DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_messages_guild ON messages(guild_id, created_at);

CREATE TABLE IF NOT EXISTS signal_prekey_bundles (
	pub_key           TEXT PRIMARY KEY,
	registration_id   INTEGER NOT NULL,
	identity_key_pub  TEXT NOT NULL,
	signed_prekey_id  INTEGER NOT NULL,
	signed_prekey_pub TEXT NOT NULL,
	signed_prekey_sig TEXT NOT NULL,
	kyber_prekey_id   INTEGER NOT NULL DEFAULT 0,
	kyber_prekey_pub  TEXT NOT NULL DEFAULT '',
	kyber_prekey_sig  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS signal_one_time_prekeys (
	pub_key   TEXT NOT NULL,
	prekey_id INTEGER NOT NULL,
	prekey_pub TEXT NOT NULL,
	PRIMARY KEY (pub_key, prekey_id)
);

CREATE TABLE IF NOT EXISTS dm_messages (
	id              TEXT PRIMARY KEY,
	sender_pub      TEXT NOT NULL,
	recipient_pub   TEXT NOT NULL,
	ciphertext_b64  TEXT NOT NULL,
	message_type    INTEGER NOT NULL DEFAULT 2,
	created_at      DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dm_sender_recip ON dm_messages(sender_pub, recipient_pub, created_at);
CREATE INDEX IF NOT EXISTS idx_dm_recip_sender ON dm_messages(recipient_pub, sender_pub, created_at);
`

// Verify DB satisfies Storage at compile time.
var _ Storage = (*DB)(nil)

type DB struct {
	db *sql.DB
}

func NewDB(dsn string) (*DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("set FK: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &DB{db: db}, nil
}

func (d *DB) Close() error { return d.db.Close() }

// RawDB returns the underlying *sql.DB for use by SQLiteGuildState.
func (d *DB) RawDB() *sql.DB { return d.db }

// --- Users ---

type DBUser struct {
	DiscordID     string
	Username      string
	AvatarURL     string
	Ed25519Pub    string
	X25519Pub     string
	EncryptedPriv []byte
	KeySalt       []byte
	KeyIterations int
}

func (d *DB) CreateUser(u DBUser) error {
	_, err := d.db.Exec(
		`INSERT INTO users (discord_id, username, avatar_url, ed25519_pub, x25519_pub, encrypted_privkey, key_salt, key_iterations)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.DiscordID, u.Username, u.AvatarURL, u.Ed25519Pub, u.X25519Pub, u.EncryptedPriv, u.KeySalt, u.KeyIterations,
	)
	return err
}

func (d *DB) GetUserByDiscordID(discordID string) (*DBUser, error) {
	row := d.db.QueryRow(
		`SELECT discord_id, username, COALESCE(avatar_url,''), ed25519_pub, x25519_pub, encrypted_privkey, key_salt, key_iterations
		 FROM users WHERE discord_id = ?`, discordID)
	var u DBUser
	if err := row.Scan(&u.DiscordID, &u.Username, &u.AvatarURL, &u.Ed25519Pub, &u.X25519Pub, &u.EncryptedPriv, &u.KeySalt, &u.KeyIterations); err != nil {
		return nil, err
	}
	return &u, nil
}

func (d *DB) UpdateUserAvatar(discordID, avatarURL string) {
	d.db.Exec("UPDATE users SET avatar_url = ? WHERE discord_id = ?", avatarURL, discordID)
}

func (d *DB) ListUsers() []protocol.User {
	rows, err := d.db.Query(`SELECT username, ed25519_pub FROM users`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []protocol.User
	for rows.Next() {
		var u protocol.User
		if err := rows.Scan(&u.Name, &u.PubKey); err != nil {
			return out
		}
		out = append(out, u)
	}
	return out
}

func (d *DB) UserExistsByPubKey(pub string) bool {
	var n int
	d.db.QueryRow(`SELECT COUNT(*) FROM users WHERE ed25519_pub = ?`, pub).Scan(&n)
	return n > 0
}

// RegisterUser implements Storage for CLI clients (no Discord SSO).
func (d *DB) RegisterUser(name, pubKey string) *protocol.User {
	d.db.Exec(
		`INSERT OR IGNORE INTO users (discord_id, username, avatar_url, ed25519_pub, x25519_pub, encrypted_privkey, key_salt, key_iterations)
		 VALUES (?, ?, '', ?, '', '', '', 0)`,
		"cli:"+pubKey, name, pubKey,
	)
	return &protocol.User{Name: name, PubKey: pubKey}
}

// UserExists implements Storage.
func (d *DB) UserExists(pubKey string) bool {
	return d.UserExistsByPubKey(pubKey)
}

// --- Sealed Keys ---

func (d *DB) StoreKeys(guildID, recipientPub string, keys []protocol.SealedKeyEntry) {
	for _, k := range keys {
		d.db.Exec(
			`INSERT OR REPLACE INTO sealed_keys (guild_id, recipient_pub, sealed_key_b64, sealer_pub)
			 VALUES (?, ?, ?, ?)`,
			guildID, recipientPub, k.SealedKeyB64, k.SealerPub,
		)
	}
}

func (d *DB) GetKeys(guildID, recipientPub string) []protocol.SealedKeyEntry {
	rows, err := d.db.Query(
		`SELECT sealed_key_b64, sealer_pub FROM sealed_keys WHERE guild_id = ? AND recipient_pub = ?`,
		guildID, recipientPub)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []protocol.SealedKeyEntry
	for rows.Next() {
		var e protocol.SealedKeyEntry
		rows.Scan(&e.SealedKeyB64, &e.SealerPub)
		out = append(out, e)
	}
	return out
}

func (d *DB) RemoveKeys(guildID, pubKey string) {
	d.db.Exec(`DELETE FROM sealed_keys WHERE guild_id = ? AND recipient_pub = ?`, guildID, pubKey)
}

// --- Messages ---

func (d *DB) StoreMessage(guildID, senderPub, ciphertextB64 string) (*protocol.MessageEnvelope, error) {
	now := time.Now()
	env := protocol.MessageEnvelope{
		ID:            uuid.New().String(),
		GuildID:       guildID,
		SenderPub:     senderPub,
		CiphertextB64: ciphertextB64,
		Timestamp:     now,
	}
	_, err := d.db.Exec(
		`INSERT INTO messages (id, guild_id, sender_pub, ciphertext_b64, created_at) VALUES (?, ?, ?, ?, ?)`,
		env.ID, guildID, senderPub, ciphertextB64, now,
	)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (d *DB) ListMessages(guildID string, after time.Time) []protocol.MessageEnvelope {
	var rows *sql.Rows
	var err error
	if after.IsZero() {
		rows, err = d.db.Query(
			`SELECT id, guild_id, sender_pub, ciphertext_b64, created_at FROM messages WHERE guild_id = ? ORDER BY created_at`,
			guildID)
	} else {
		rows, err = d.db.Query(
			`SELECT id, guild_id, sender_pub, ciphertext_b64, created_at FROM messages WHERE guild_id = ? AND created_at > ? ORDER BY created_at`,
			guildID, after)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []protocol.MessageEnvelope
	for rows.Next() {
		var m protocol.MessageEnvelope
		rows.Scan(&m.ID, &m.GuildID, &m.SenderPub, &m.CiphertextB64, &m.Timestamp)
		out = append(out, m)
	}
	return out
}

// --- Signal Pre-Keys ---

type SignalPreKeyBundle struct {
	PubKey          string
	RegistrationID  int
	IdentityKeyPub  string
	SignedPreKeyID  int
	SignedPreKeyPub string
	SignedPreKeySig string
	KyberPreKeyID   int
	KyberPreKeyPub  string
	KyberPreKeySig  string
}

type SignalOneTimePreKey struct {
	PubKey    string
	PreKeyID  int
	PreKeyPub string
}

func (d *DB) StoreSignalPreKeyBundle(b SignalPreKeyBundle) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO signal_prekey_bundles
		 (pub_key, registration_id, identity_key_pub, signed_prekey_id, signed_prekey_pub, signed_prekey_sig, kyber_prekey_id, kyber_prekey_pub, kyber_prekey_sig)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.PubKey, b.RegistrationID, b.IdentityKeyPub, b.SignedPreKeyID, b.SignedPreKeyPub, b.SignedPreKeySig,
		b.KyberPreKeyID, b.KyberPreKeyPub, b.KyberPreKeySig,
	)
	return err
}

func (d *DB) StoreSignalOneTimePreKeys(pubKey string, keys []SignalOneTimePreKey) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(
		`INSERT OR REPLACE INTO signal_one_time_prekeys (pub_key, prekey_id, prekey_pub) VALUES (?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, k := range keys {
		stmt.Exec(pubKey, k.PreKeyID, k.PreKeyPub)
	}
	return tx.Commit()
}

func (d *DB) GetSignalPreKeyBundle(pubKey string) (*SignalPreKeyBundle, *SignalOneTimePreKey, error) {
	row := d.db.QueryRow(
		`SELECT pub_key, registration_id, identity_key_pub, signed_prekey_id, signed_prekey_pub, signed_prekey_sig, kyber_prekey_id, kyber_prekey_pub, kyber_prekey_sig
		 FROM signal_prekey_bundles WHERE pub_key = ?`, pubKey)
	var b SignalPreKeyBundle
	if err := row.Scan(&b.PubKey, &b.RegistrationID, &b.IdentityKeyPub, &b.SignedPreKeyID, &b.SignedPreKeyPub, &b.SignedPreKeySig, &b.KyberPreKeyID, &b.KyberPreKeyPub, &b.KyberPreKeySig); err != nil {
		return nil, nil, err
	}

	// Consume one one-time prekey (FIFO)
	otkRow := d.db.QueryRow(
		`SELECT pub_key, prekey_id, prekey_pub FROM signal_one_time_prekeys WHERE pub_key = ? LIMIT 1`, pubKey)
	var otk SignalOneTimePreKey
	if err := otkRow.Scan(&otk.PubKey, &otk.PreKeyID, &otk.PreKeyPub); err == nil {
		d.db.Exec(`DELETE FROM signal_one_time_prekeys WHERE pub_key = ? AND prekey_id = ?`, pubKey, otk.PreKeyID)
		return &b, &otk, nil
	}
	return &b, nil, nil
}

// --- DM Messages ---

func (d *DB) StoreDMMessage(senderPub, recipientPub, ciphertextB64 string, messageType int) (*protocol.DMMessage, error) {
	now := time.Now()
	msg := protocol.DMMessage{
		ID:            uuid.New().String(),
		SenderPub:     senderPub,
		RecipientPub:  recipientPub,
		CiphertextB64: ciphertextB64,
		MessageType:   messageType,
		Timestamp:     now,
	}
	_, err := d.db.Exec(
		`INSERT INTO dm_messages (id, sender_pub, recipient_pub, ciphertext_b64, message_type, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		msg.ID, senderPub, recipientPub, ciphertextB64, messageType, now,
	)
	if err != nil {
		return nil, err
	}
	return &msg, nil
}

func (d *DB) ListDMMessages(pub1, pub2 string, after time.Time) []protocol.DMMessage {
	var rows *sql.Rows
	var err error
	if after.IsZero() {
		rows, err = d.db.Query(
			`SELECT id, sender_pub, recipient_pub, ciphertext_b64, message_type, created_at FROM dm_messages
			 WHERE (sender_pub = ? AND recipient_pub = ?) OR (sender_pub = ? AND recipient_pub = ?)
			 ORDER BY created_at`,
			pub1, pub2, pub2, pub1)
	} else {
		rows, err = d.db.Query(
			`SELECT id, sender_pub, recipient_pub, ciphertext_b64, message_type, created_at FROM dm_messages
			 WHERE ((sender_pub = ? AND recipient_pub = ?) OR (sender_pub = ? AND recipient_pub = ?))
			 AND created_at > ?
			 ORDER BY created_at`,
			pub1, pub2, pub2, pub1, after)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []protocol.DMMessage
	for rows.Next() {
		var m protocol.DMMessage
		rows.Scan(&m.ID, &m.SenderPub, &m.RecipientPub, &m.CiphertextB64, &m.MessageType, &m.Timestamp)
		out = append(out, m)
	}
	return out
}

func (d *DB) ListDMConversations(pubKey string) []string {
	rows, err := d.db.Query(
		`SELECT DISTINCT CASE WHEN sender_pub = ? THEN recipient_pub ELSE sender_pub END as peer
		 FROM dm_messages WHERE sender_pub = ? OR recipient_pub = ?`,
		pubKey, pubKey, pubKey)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var peer string
		rows.Scan(&peer)
		out = append(out, peer)
	}
	return out
}

// --- Guild member avatars (encrypted per-guild) ---

func (d *DB) SetGuildMemberAvatar(guildID, memberPub, encryptedAvatarB64 string) error {
	_, err := d.db.Exec(
		`INSERT OR REPLACE INTO guild_member_avatars (guild_id, member_pub, encrypted_avatar_b64) VALUES (?, ?, ?)`,
		guildID, memberPub, encryptedAvatarB64)
	return err
}

func (d *DB) GetGuildMemberAvatars(guildID string) ([]GuildMemberAvatar, error) {
	rows, err := d.db.Query(
		`SELECT member_pub, encrypted_avatar_b64 FROM guild_member_avatars WHERE guild_id = ?`,
		guildID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GuildMemberAvatar
	for rows.Next() {
		var a GuildMemberAvatar
		if err := rows.Scan(&a.MemberPub, &a.EncryptedAvatarB64); err != nil {
			return out, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

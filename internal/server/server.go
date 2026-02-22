package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/anthropic/angelcord/internal/guild"
	"github.com/anthropic/angelcord/internal/protocol"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter returns an http.Handler with all server API routes.
// It takes a GuildState (governance, swappable for Solana) and a DB
// (SQLite persistence for users, messages, sealed keys, DMs).
func NewRouter(gs guild.GuildState, store Storage, ws ...*WSHub) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	var hub *WSHub
	if len(ws) > 0 {
		hub = ws[0]
	}
	h := &handlers{gs: gs, store: store, ws: hub}

	r.Route("/api", func(r chi.Router) {
		r.Post("/register", h.register)

		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware)

			r.Get("/users", h.listUsers)

			r.Post("/guilds", h.createGuild)
			r.Get("/guilds", h.listGuilds)
			r.Get("/guilds/{guildID}", h.getGuild)

			r.Post("/guilds/{guildID}/invite", h.invite)
			r.Delete("/guilds/{guildID}/members/{pubkey}", h.kick)
			r.Get("/guilds/{guildID}/members", h.listMembers)

			r.Post("/guilds/{guildID}/keys", h.uploadKeys)
			r.Get("/guilds/{guildID}/keys", h.getKeys)

			r.Post("/guilds/{guildID}/avatars", h.uploadAvatar)
			r.Get("/guilds/{guildID}/avatars", h.getAvatars)

			r.Post("/guilds/{guildID}/messages", h.sendMessage)
			r.Get("/guilds/{guildID}/messages", h.getMessages)

			// DM endpoints
			r.Post("/dms/{pubkey}/messages", h.sendDM)
			r.Get("/dms/{pubkey}/messages", h.getDMs)
			r.Get("/dms", h.listDMConversations)

			// Signal pre-key endpoints
			r.Post("/signal/prekeys", h.uploadPreKeys)
			r.Get("/signal/prekeys/{pubkey}", h.getPreKeys)
		})
	})

	return r
}

// Storage abstracts the persistence layer so the server works
// with both the in-memory RelayStore (tests/CLI) and SQLite DB.
type Storage interface {
	RegisterUser(name, pubKey string) *protocol.User
	UserExists(pubKey string) bool
	ListUsers() []protocol.User
	StoreKeys(guildID, recipientPub string, keys []protocol.SealedKeyEntry)
	GetKeys(guildID, recipientPub string) []protocol.SealedKeyEntry
	RemoveKeys(guildID, pubKey string)
	StoreMessage(guildID, senderPub, ciphertextB64 string) (*protocol.MessageEnvelope, error)
	ListMessages(guildID string, after time.Time) []protocol.MessageEnvelope
	StoreDMMessage(senderPub, recipientPub, ciphertextB64 string, messageType int) (*protocol.DMMessage, error)
	ListDMMessages(pub1, pub2 string, after time.Time) []protocol.DMMessage
	ListDMConversations(pubKey string) []string
	StoreSignalPreKeyBundle(b SignalPreKeyBundle) error
	StoreSignalOneTimePreKeys(pubKey string, keys []SignalOneTimePreKey) error
	GetSignalPreKeyBundle(pubKey string) (*SignalPreKeyBundle, *SignalOneTimePreKey, error)

	SetGuildMemberAvatar(guildID, memberPub, encryptedAvatarB64 string) error
	GetGuildMemberAvatars(guildID string) ([]GuildMemberAvatar, error)
}

type GuildMemberAvatar struct {
	MemberPub          string `json:"member_pub"`
	EncryptedAvatarB64 string `json:"encrypted_avatar_b64"`
}

type handlers struct {
	gs    guild.GuildState
	store Storage
	ws    *WSHub
}

// ---------------------------------------------------------------------------
// JSON helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, protocol.ErrorResponse{Error: msg})
}

func decodeBody(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	var req protocol.RegisterRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.PubKey == "" {
		writeErr(w, http.StatusBadRequest, "name and pub_key are required")
		return
	}
	if h.store.UserExists(req.PubKey) {
		writeErr(w, http.StatusConflict, "public key already registered")
		return
	}
	u := h.store.RegisterUser(req.Name, req.PubKey)
	writeJSON(w, http.StatusCreated, u)
}

func (h *handlers) listUsers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.store.ListUsers())
}

// ---------------------------------------------------------------------------
// Guilds
// ---------------------------------------------------------------------------

func (h *handlers) createGuild(w http.ResponseWriter, r *http.Request) {
	var req protocol.CreateGuildRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	ownerPub := getPubKeyB64(r)
	guildID, err := h.gs.CreateGuild(req.Name, ownerPub)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	info, _ := h.gs.GetGuild(guildID)
	writeJSON(w, http.StatusCreated, guildInfoToResponse(info))
}

func (h *handlers) listGuilds(w http.ResponseWriter, r *http.Request) {
	pubKey := getPubKeyB64(r)
	infos, err := h.gs.ListGuildsForUser(pubKey)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]protocol.GuildResponse, len(infos))
	for i, info := range infos {
		out[i] = guildInfoToResponse(&info)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *handlers) getGuild(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	pubKey := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, pubKey)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	info, err := h.gs.GetGuild(guildID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, guildInfoToResponse(info))
}

func guildInfoToResponse(info *guild.GuildInfo) protocol.GuildResponse {
	return protocol.GuildResponse{
		ID:       info.ID,
		Name:     info.Name,
		OwnerPub: info.OwnerPubKey,
		Members:  info.Members,
	}
}

// ---------------------------------------------------------------------------
// Members / Invite
// ---------------------------------------------------------------------------

func (h *handlers) invite(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	inviterPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, inviterPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	var req protocol.InviteRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InviteePubKey == "" {
		writeErr(w, http.StatusBadRequest, "invitee_pub_key is required")
		return
	}

	if err := h.gs.AddMember(guildID, req.InviteePubKey); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	if len(req.SealedKeys) > 0 {
		h.store.StoreKeys(guildID, req.InviteePubKey, req.SealedKeys)
	}

	if h.ws != nil {
		h.ws.BroadcastMemberEvent(guildID, "member_joined", req.InviteePubKey)
		info, _ := h.gs.GetGuild(guildID)
		if info != nil {
			h.ws.BroadcastToUser(req.InviteePubKey, map[string]any{
				"type":  "guild_added",
				"guild": guildInfoToResponse(info),
			})
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "invited"})
}

func (h *handlers) kick(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	targetPub := urlSafeToStdB64(chi.URLParam(r, "pubkey"))
	kickerPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, kickerPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	if err := h.gs.RemoveMember(guildID, targetPub); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	h.store.RemoveKeys(guildID, targetPub)

	writeJSON(w, http.StatusOK, map[string]string{"status": "kicked"})
}

func urlSafeToStdB64(s string) string {
	r := strings.NewReplacer("-", "+", "_", "/")
	return r.Replace(s)
}

func (h *handlers) listMembers(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	pubKey := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, pubKey)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	members, err := h.gs.ListMembers(guildID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// ---------------------------------------------------------------------------
// Guild Key Bundles
// ---------------------------------------------------------------------------

func (h *handlers) uploadKeys(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	uploaderPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, uploaderPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	var req protocol.UploadKeysRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	for _, bundle := range req.Bundles {
		h.store.StoreKeys(guildID, bundle.RecipientPub, bundle.Keys)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) getKeys(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	requesterPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, requesterPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	keys := h.store.GetKeys(guildID, requesterPub)
	writeJSON(w, http.StatusOK, protocol.KeyBundle{
		GuildID:      guildID,
		RecipientPub: requesterPub,
		Keys:         keys,
	})
}

func (h *handlers) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	memberPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, memberPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	var req struct {
		EncryptedAvatarB64 string `json:"encrypted_avatar_b64"`
	}
	if err := decodeBody(r, &req); err != nil || req.EncryptedAvatarB64 == "" {
		writeErr(w, http.StatusBadRequest, "encrypted_avatar_b64 required")
		return
	}

	if err := h.store.SetGuildMemberAvatar(guildID, memberPub, req.EncryptedAvatarB64); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) getAvatars(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	requesterPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, requesterPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	avatars, err := h.store.GetGuildMemberAvatars(guildID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"avatars": avatars})
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func (h *handlers) sendMessage(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	senderPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, senderPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	var req protocol.SendMessageRequest
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	env, err := h.store.StoreMessage(guildID, senderPub, req.CiphertextB64)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.ws != nil {
		h.ws.BroadcastGuildMessage(guildID, env)
	}
	writeJSON(w, http.StatusCreated, env)
}

func (h *handlers) getMessages(w http.ResponseWriter, r *http.Request) {
	guildID := chi.URLParam(r, "guildID")
	requesterPub := getPubKeyB64(r)

	isMember, _ := h.gs.IsMember(guildID, requesterPub)
	if !isMember {
		writeErr(w, http.StatusForbidden, "not a member of this guild")
		return
	}

	var after time.Time
	if q := r.URL.Query().Get("after"); q != "" {
		if t, err := time.Parse(time.RFC3339Nano, q); err == nil {
			after = t
		}
	}
	writeJSON(w, http.StatusOK, h.store.ListMessages(guildID, after))
}

// ---------------------------------------------------------------------------
// DMs
// ---------------------------------------------------------------------------

func (h *handlers) sendDM(w http.ResponseWriter, r *http.Request) {
	senderPub := getPubKeyB64(r)
	recipientPub := urlSafeToStdB64(chi.URLParam(r, "pubkey"))

	var req struct {
		CiphertextB64 string `json:"ciphertext_b64"`
		MessageType   int    `json:"message_type"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	messageType := req.MessageType
	if messageType == 0 {
		messageType = 2 // default: Signal message
	}

	msg, err := h.store.StoreDMMessage(senderPub, recipientPub, req.CiphertextB64, messageType)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.ws != nil {
		h.ws.BroadcastDM(recipientPub, msg)
	}
	writeJSON(w, http.StatusCreated, msg)
}

func (h *handlers) getDMs(w http.ResponseWriter, r *http.Request) {
	requesterPub := getPubKeyB64(r)
	peerPub := urlSafeToStdB64(chi.URLParam(r, "pubkey"))

	var after time.Time
	if q := r.URL.Query().Get("after"); q != "" {
		if t, err := time.Parse(time.RFC3339Nano, q); err == nil {
			after = t
		}
	}
	msgs := h.store.ListDMMessages(requesterPub, peerPub, after)
	writeJSON(w, http.StatusOK, msgs)
}

func (h *handlers) listDMConversations(w http.ResponseWriter, r *http.Request) {
	pubKey := getPubKeyB64(r)
	peers := h.store.ListDMConversations(pubKey)
	writeJSON(w, http.StatusOK, peers)
}

// ---------------------------------------------------------------------------
// Signal Pre-Keys
// ---------------------------------------------------------------------------

func (h *handlers) uploadPreKeys(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PubKey          string `json:"pub_key"`
		RegistrationID  int    `json:"registration_id"`
		IdentityKeyPub  string `json:"identity_key_pub"`
		SignedPreKeyID  int    `json:"signed_prekey_id"`
		SignedPreKeyPub string `json:"signed_prekey_pub"`
		SignedPreKeySig string `json:"signed_prekey_sig"`
		OneTimePreKeys  []struct {
			PreKeyID  int    `json:"prekey_id"`
			PreKeyPub string `json:"prekey_pub"`
		} `json:"one_time_prekeys"`
		KyberPreKeyID  int    `json:"kyber_prekey_id"`
		KyberPreKeyPub string `json:"kyber_prekey_pub"`
		KyberPreKeySig string `json:"kyber_prekey_sig"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	bundle := SignalPreKeyBundle{
		PubKey:          req.PubKey,
		RegistrationID:  req.RegistrationID,
		IdentityKeyPub:  req.IdentityKeyPub,
		SignedPreKeyID:  req.SignedPreKeyID,
		SignedPreKeyPub: req.SignedPreKeyPub,
		SignedPreKeySig: req.SignedPreKeySig,
		KyberPreKeyID:   req.KyberPreKeyID,
		KyberPreKeyPub:  req.KyberPreKeyPub,
		KyberPreKeySig:  req.KyberPreKeySig,
	}
	if err := h.store.StoreSignalPreKeyBundle(bundle); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	if len(req.OneTimePreKeys) > 0 {
		otks := make([]SignalOneTimePreKey, len(req.OneTimePreKeys))
		for i, k := range req.OneTimePreKeys {
			otks[i] = SignalOneTimePreKey{PubKey: req.PubKey, PreKeyID: k.PreKeyID, PreKeyPub: k.PreKeyPub}
		}
		if err := h.store.StoreSignalOneTimePreKeys(req.PubKey, otks); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handlers) getPreKeys(w http.ResponseWriter, r *http.Request) {
	pubKey := urlSafeToStdB64(chi.URLParam(r, "pubkey"))
	bundle, otk, err := h.store.GetSignalPreKeyBundle(pubKey)
	if err != nil {
		writeErr(w, http.StatusNotFound, "no pre-key bundle found")
		return
	}

	resp := map[string]any{
		"pub_key":           bundle.PubKey,
		"registration_id":   bundle.RegistrationID,
		"identity_key_pub":  bundle.IdentityKeyPub,
		"signed_prekey_id":  bundle.SignedPreKeyID,
		"signed_prekey_pub": bundle.SignedPreKeyPub,
		"signed_prekey_sig": bundle.SignedPreKeySig,
		"kyber_prekey_id":   bundle.KyberPreKeyID,
		"kyber_prekey_pub":  bundle.KyberPreKeyPub,
		"kyber_prekey_sig":  bundle.KyberPreKeySig,
	}
	if otk != nil {
		resp["one_time_prekey"] = map[string]any{
			"prekey_id":  otk.PreKeyID,
			"prekey_pub": otk.PreKeyPub,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

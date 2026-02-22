package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// WSHub manages WebSocket connections and broadcasts.
type WSHub struct {
	mu      sync.RWMutex
	clients map[*WSClient]bool
}

type WSClient struct {
	hub    *WSHub
	conn   *websocket.Conn
	pubKey string
	guilds map[string]bool
	sendCh chan []byte
}

type WSMessage struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
	GuildID string          `json:"guild_id,omitempty"`
	From    string          `json:"from,omitempty"`
}

func NewWSHub() *WSHub {
	return &WSHub{
		clients: make(map[*WSClient]bool),
	}
}

func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("ws upgrade: %v", err)
		return
	}

	pubKey := ""
	if claims, ok := getJWTClaims(r); ok {
		pubKey, _ = claims["pub_key"].(string)
	}

	client := &WSClient{
		hub:    h,
		conn:   conn,
		pubKey: pubKey,
		guilds: make(map[string]bool),
		sendCh: make(chan []byte, 256),
	}

	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()

	go client.writePump()
	go client.readPump()
}

func (c *WSClient) readPump() {
	defer func() {
		c.hub.mu.Lock()
		delete(c.hub.clients, c)
		c.hub.mu.Unlock()
		c.conn.Close()
	}()
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		var msg WSMessage
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case "subscribe":
			c.guilds[msg.GuildID] = true
		case "unsubscribe":
			delete(c.guilds, msg.GuildID)
		}
	}
}

func (c *WSClient) writePump() {
	defer c.conn.Close()
	for msg := range c.sendCh {
		if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			break
		}
	}
}

// BroadcastGuildMessage sends a message to all clients subscribed to a guild.
func (h *WSHub) BroadcastGuildMessage(guildID string, payload any) {
	data, err := json.Marshal(map[string]any{
		"type":     "guild_message",
		"guild_id": guildID,
		"envelope": payload,
	})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.guilds[guildID] {
			select {
			case client.sendCh <- data:
			default:
			}
		}
	}
}

// BroadcastDM sends a DM to the recipient's WebSocket connection.
func (h *WSHub) BroadcastDM(recipientPub string, payload any) {
	data, err := json.Marshal(map[string]any{
		"type":    "dm_message",
		"message": payload,
	})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.pubKey == recipientPub {
			select {
			case client.sendCh <- data:
			default:
			}
		}
	}
}

// BroadcastToUser sends a message directly to a specific user by pubkey.
func (h *WSHub) BroadcastToUser(pubKey string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.pubKey == pubKey {
			select {
			case client.sendCh <- data:
			default:
			}
		}
	}
}

// BroadcastMemberEvent sends member join/leave events to guild subscribers.
func (h *WSHub) BroadcastMemberEvent(guildID, eventType, pubKey string) {
	data, err := json.Marshal(map[string]any{
		"type":     eventType,
		"guild_id": guildID,
		"pub_key":  pubKey,
	})
	if err != nil {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.guilds[guildID] {
			select {
			case client.sendCh <- data:
			default:
			}
		}
	}
}

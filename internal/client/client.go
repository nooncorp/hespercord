// Package client provides the HTTP client that talks to the angelcord server
// and the local crypto session that manages guild keys.
package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/anthropic/angelcord/internal/crypto"
	"github.com/anthropic/angelcord/internal/protocol"
)

// HTTPClient wraps all REST calls to the angelcord server, with Ed25519
// request signing for authenticated endpoints.
type HTTPClient struct {
	baseURL  string
	http     *http.Client
	identity *crypto.Identity
}

func NewHTTPClient(baseURL string, identity *crypto.Identity) *HTTPClient {
	return &HTTPClient{
		baseURL:  baseURL,
		http:     &http.Client{Timeout: 10 * time.Second},
		identity: identity,
	}
}

// ---------------------------------------------------------------------------
// generic helpers
// ---------------------------------------------------------------------------

func (c *HTTPClient) doJSON(method, path string, body any, dst any) error {
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	var bodyReader io.Reader
	if bodyBytes != nil {
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if c.identity != nil {
		req.Header.Set("X-PubKey", c.identity.PubKeyBase64())
		if method != http.MethodGet {
			sig := c.identity.SignRequest(method, path, bodyBytes)
			req.Header.Set("X-Signature", base64.StdEncoding.EncodeToString(sig))
		}
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		var errResp protocol.ErrorResponse
		if json.Unmarshal(respBytes, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("server: %s", errResp.Error)
		}
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(respBytes))
	}

	if dst != nil {
		return json.Unmarshal(respBytes, dst)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func (c *HTTPClient) Register(name, pubKey string) (*protocol.User, error) {
	var u protocol.User
	err := c.doJSON("POST", "/api/register", protocol.RegisterRequest{
		Name: name, PubKey: pubKey,
	}, &u)
	return &u, err
}

func (c *HTTPClient) ListUsers() ([]protocol.User, error) {
	var users []protocol.User
	err := c.doJSON("GET", "/api/users", nil, &users)
	return users, err
}

// ---------------------------------------------------------------------------
// Guilds
// ---------------------------------------------------------------------------

func (c *HTTPClient) CreateGuild(name string) (*protocol.GuildResponse, error) {
	var g protocol.GuildResponse
	err := c.doJSON("POST", "/api/guilds", protocol.CreateGuildRequest{Name: name}, &g)
	return &g, err
}

func (c *HTTPClient) ListGuilds() ([]protocol.GuildResponse, error) {
	var gs []protocol.GuildResponse
	err := c.doJSON("GET", "/api/guilds", nil, &gs)
	return gs, err
}

func (c *HTTPClient) GetGuild(id string) (*protocol.GuildResponse, error) {
	var g protocol.GuildResponse
	err := c.doJSON("GET", "/api/guilds/"+id, nil, &g)
	return &g, err
}

// ---------------------------------------------------------------------------
// Members
// ---------------------------------------------------------------------------

func (c *HTTPClient) Invite(guildID string, req protocol.InviteRequest) error {
	return c.doJSON("POST", "/api/guilds/"+guildID+"/invite", req, nil)
}

func (c *HTTPClient) Kick(guildID, targetPubB64 string) error {
	urlSafe := stdB64ToURLSafe(targetPubB64)
	path := "/api/guilds/" + guildID + "/members/" + urlSafe
	return c.doJSON("DELETE", path, nil, nil)
}

func stdB64ToURLSafe(s string) string {
	r := strings.NewReplacer("+", "-", "/", "_")
	return r.Replace(s)
}

func (c *HTTPClient) ListMembers(guildID string) ([]string, error) {
	var members []string
	err := c.doJSON("GET", "/api/guilds/"+guildID+"/members", nil, &members)
	return members, err
}

// ---------------------------------------------------------------------------
// Guild Keys
// ---------------------------------------------------------------------------

func (c *HTTPClient) UploadKeys(guildID string, req protocol.UploadKeysRequest) error {
	return c.doJSON("POST", "/api/guilds/"+guildID+"/keys", req, nil)
}

func (c *HTTPClient) GetKeys(guildID string) (*protocol.KeyBundle, error) {
	var bundle protocol.KeyBundle
	err := c.doJSON("GET", "/api/guilds/"+guildID+"/keys", nil, &bundle)
	return &bundle, err
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

func (c *HTTPClient) SendMessage(guildID string, req protocol.SendMessageRequest) (*protocol.MessageEnvelope, error) {
	var env protocol.MessageEnvelope
	err := c.doJSON("POST", "/api/guilds/"+guildID+"/messages", req, &env)
	return &env, err
}

func (c *HTTPClient) GetMessages(guildID string, after time.Time) ([]protocol.MessageEnvelope, error) {
	path := "/api/guilds/" + guildID + "/messages"
	if !after.IsZero() {
		path += "?after=" + url.QueryEscape(after.Format(time.RFC3339Nano))
	}
	var msgs []protocol.MessageEnvelope
	err := c.doJSON("GET", path, nil, &msgs)
	return msgs, err
}

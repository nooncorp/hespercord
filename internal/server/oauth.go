package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		b := make([]byte, 32)
		rand.Read(b)
		secret = hex.EncodeToString(b)
	}
	jwtSecret = []byte(secret)
}

type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func NewOAuthConfig() OAuthConfig {
	return OAuthConfig{
		ClientID:     os.Getenv("DISCORD_CLIENT_ID"),
		ClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		RedirectURL:  os.Getenv("DISCORD_REDIRECT_URL"),
	}
}

type oauthHandlers struct {
	cfg OAuthConfig
	db  *DB
}

// MountOAuthRoutes adds /auth/* routes to the given chi router.
func MountOAuthRoutes(r chi.Router, cfg OAuthConfig, db *DB) {
	o := &oauthHandlers{cfg: cfg, db: db}
	r.Route("/auth", func(r chi.Router) {
		r.Get("/discord", o.discordRedirect)
		r.Get("/discord/callback", o.discordCallback)
		r.Get("/me", o.me)
		r.Post("/signup", o.signup)
		r.Post("/logout", o.logout)
	})
}

func (o *oauthHandlers) discordRedirect(w http.ResponseWriter, r *http.Request) {
	state := generateState()
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	params := url.Values{
		"client_id":     {o.cfg.ClientID},
		"redirect_uri":  {o.cfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {"identify"},
		"state":         {state},
	}
	http.Redirect(w, r, "https://discord.com/api/oauth2/authorize?"+params.Encode(), http.StatusTemporaryRedirect)
}

func (o *oauthHandlers) discordCallback(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}

	tokenResp, err := http.PostForm("https://discord.com/api/oauth2/token", url.Values{
		"client_id":     {o.cfg.ClientID},
		"client_secret": {o.cfg.ClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {o.cfg.RedirectURL},
	})
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusInternalServerError)
		return
	}
	defer tokenResp.Body.Close()

	var tokenData struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenData); err != nil || tokenData.AccessToken == "" {
		http.Error(w, "invalid token response", http.StatusInternalServerError)
		return
	}

	userReq, _ := http.NewRequest("GET", "https://discord.com/api/users/@me", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenData.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		http.Error(w, "user fetch failed", http.StatusInternalServerError)
		return
	}
	defer userResp.Body.Close()

	var discordUser struct {
		ID       string  `json:"id"`
		Username string  `json:"username"`
		Avatar   *string `json:"avatar"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&discordUser); err != nil {
		http.Error(w, "invalid user response", http.StatusInternalServerError)
		return
	}
	avatarURL := discordAvatarURL(discordUser.ID, discordUser.Avatar)

	dbUser, err := o.db.GetUserByDiscordID(discordUser.ID)
	if err != nil {
		token := issueJWT(discordUser.ID, discordUser.Username, "", true, avatarURL)
		http.SetCookie(w, &http.Cookie{
			Name:     "session",
			Value:    token,
			Path:     "/",
			MaxAge:   86400 * 7,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		http.Redirect(w, r, "/app/signup", http.StatusTemporaryRedirect)
		return
	}

	o.db.UpdateUserAvatar(discordUser.ID, avatarURL)
	token := issueJWT(discordUser.ID, discordUser.Username, dbUser.Ed25519Pub, false, "")
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/app", http.StatusTemporaryRedirect)
}

func (o *oauthHandlers) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := getJWTClaims(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	discordID := claims["discord_id"].(string)
	username := claims["username"].(string)
	isNewUser, _ := claims["is_new_user"].(bool)

	if isNewUser {
		writeJSON(w, http.StatusOK, map[string]any{
			"discord_id": discordID,
			"username":   username,
			"is_new":     true,
		})
		return
	}

	dbUser, err := o.db.GetUserByDiscordID(discordID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"discord_id":        dbUser.DiscordID,
		"username":          dbUser.Username,
		"avatar_url":        dbUser.AvatarURL,
		"ed25519_pub":       dbUser.Ed25519Pub,
		"x25519_pub":        dbUser.X25519Pub,
		"encrypted_privkey": base64.StdEncoding.EncodeToString(dbUser.EncryptedPriv),
		"key_salt":          base64.StdEncoding.EncodeToString(dbUser.KeySalt),
		"key_iterations":    dbUser.KeyIterations,
		"is_new":            false,
	})
}

func (o *oauthHandlers) signup(w http.ResponseWriter, r *http.Request) {
	claims, ok := getJWTClaims(r)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	discordID := claims["discord_id"].(string)
	username := claims["username"].(string)

	var req struct {
		Ed25519Pub    string `json:"ed25519_pub"`
		X25519Pub     string `json:"x25519_pub"`
		EncryptedPriv string `json:"encrypted_privkey"` // base64
		KeySalt       string `json:"key_salt"`          // base64
		KeyIterations int    `json:"key_iterations"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}

	encPriv, err := base64.StdEncoding.DecodeString(req.EncryptedPriv)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid encrypted_privkey")
		return
	}
	salt, err := base64.StdEncoding.DecodeString(req.KeySalt)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid key_salt")
		return
	}

	avatarURL, _ := claims["avatar_url"].(string)
	if err := o.db.CreateUser(DBUser{
		DiscordID:     discordID,
		Username:      username,
		AvatarURL:     avatarURL,
		Ed25519Pub:    req.Ed25519Pub,
		X25519Pub:     req.X25519Pub,
		EncryptedPriv: encPriv,
		KeySalt:       salt,
		KeyIterations: req.KeyIterations,
	}); err != nil {
		writeErr(w, http.StatusConflict, "user already exists")
		return
	}

	newToken := issueJWT(discordID, username, req.Ed25519Pub, false, "")
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    newToken,
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (o *oauthHandlers) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

// --- Discord avatar CDN URL ---

func discordAvatarURL(discordID string, avatarHash *string) string {
	if avatarHash != nil && *avatarHash != "" {
		ext := "png"
		if len(*avatarHash) > 2 && (*avatarHash)[:2] == "a_" {
			ext = "gif"
		}
		return "https://cdn.discordapp.com/avatars/" + discordID + "/" + *avatarHash + "." + ext
	}
	// Default avatar: index 0–5 (Discord uses (user_id >> 22) % 6 for embed avatars)
	var idNum int64
	for _, c := range discordID {
		if c >= '0' && c <= '9' {
			idNum = idNum*10 + int64(c-'0')
		}
	}
	if idNum < 0 {
		idNum = -idNum
	}
	idx := int(idNum >> 22 % 6)
	return "https://cdn.discordapp.com/embed/avatars/" + fmt.Sprintf("%d", idx) + ".png"
}

// --- JWT helpers ---

func issueJWT(discordID, username, pubKey string, isNewUser bool, avatarURL string) string {
	claims := jwt.MapClaims{
		"discord_id":  discordID,
		"username":    username,
		"pub_key":     pubKey,
		"is_new_user": isNewUser,
		"exp":         time.Now().Add(7 * 24 * time.Hour).Unix(),
		"iat":         time.Now().Unix(),
	}
	if avatarURL != "" {
		claims["avatar_url"] = avatarURL
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString(jwtSecret)
	return signed
}

func getJWTClaims(r *http.Request) (jwt.MapClaims, bool) {
	var tokenStr string

	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		tokenStr = strings.TrimPrefix(auth, "Bearer ")
	} else if cookie, err := r.Cookie("session"); err == nil {
		tokenStr = cookie.Value
	}

	if tokenStr == "" {
		return nil, false
	}

	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return jwtSecret, nil
	})
	if err != nil || !token.Valid {
		return nil, false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	return claims, ok
}

func generateState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

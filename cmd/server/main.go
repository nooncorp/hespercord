package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/anthropic/angelcord/internal/guild"
	"github.com/anthropic/angelcord/internal/server"
	"github.com/go-chi/chi/v5"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", "hespercord.db", "SQLite database path")
	webDir := flag.String("web", "", "path to web/dist (built React SPA)")
	flag.Parse()

	db, err := server.NewDB(*dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer db.Close()

	gs := guild.NewSQLiteGuildState(db.RawDB())
	wsHub := server.NewWSHub()

	apiRouter := server.NewRouter(gs, db, wsHub)

	r := chi.NewRouter()

	// API routes: use Handle so the apiRouter (which internally routes
	// /api/*) receives the full path and matches correctly.
	r.Handle("/api/*", apiRouter)
	r.Handle("/api", apiRouter)

	// Auth routes (OAuth2)
	oauthCfg := server.NewOAuthConfig()
	server.MountOAuthRoutes(r, oauthCfg, db)

	// WebSocket
	r.Get("/ws", wsHub.HandleWS)

	// Serve SPA — registered after API so chi prefers the more specific /api/*
	spaDir := *webDir
	if spaDir == "" {
		candidates := []string{"web/dist", "../web/dist"}
		for _, c := range candidates {
			if info, err := os.Stat(c); err == nil && info.IsDir() {
				spaDir, _ = filepath.Abs(c)
				fmt.Printf("serving SPA from %s\n", spaDir)
				break
			}
		}
	}

	if spaDir != "" {
		serveSPA(r, spaDir)
	}

	fmt.Println("=== hespercord server ===")
	fmt.Println("encrypted relay -- stores ciphertext, never sees plaintext")
	fmt.Println("guild state: SQLite (swap for Solana later)")
	if oauthCfg.ClientID != "" {
		fmt.Println("discord SSO: enabled")
	} else {
		fmt.Println("discord SSO: disabled (set DISCORD_CLIENT_ID, DISCORD_CLIENT_SECRET, DISCORD_REDIRECT_URL)")
	}
	fmt.Printf("listening on %s\n\n", *addr)

	log.Fatal(http.ListenAndServe(*addr, r))
}

func serveSPA(r chi.Router, dir string) {
	fs := http.FileServer(http.Dir(dir))

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		// Serve static file if it exists (hashed assets are long-cached by Vite)
		fpath := filepath.Join(dir, filepath.Clean(r.URL.Path))
		if info, err := os.Stat(fpath); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		// Fall back to index.html for client-side routing — never cache
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

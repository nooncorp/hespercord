package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/anthropic/angelcord/internal/client"
)

func main() {
	name := flag.String("name", "", "display name for this user (required)")
	serverURL := flag.String("server", "http://localhost:8080", "angelcord server URL")
	flag.Parse()

	if *name == "" {
		fmt.Fprintln(os.Stderr, "error: --name is required")
		fmt.Fprintln(os.Stderr, "usage: client --name alice [--server http://localhost:8080]")
		os.Exit(1)
	}

	fmt.Println("=== angelcord client ===")
	fmt.Printf("connecting to %s as %q...\n", *serverURL, *name)

	sess, err := client.NewSession(*name, *serverURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	client.RunREPL(sess)
}

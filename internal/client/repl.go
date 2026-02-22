package client

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/google/uuid"
)

// RunREPL starts an interactive command loop for the given session.
func RunREPL(sess *Session) {
	fmt.Println()
	fmt.Printf("logged in as %s (%s)\n", sess.User.Name, sess.PubKeyB64()[:16]+"...")
	fmt.Printf("public key: %s\n", sess.PubKeyB64())
	fmt.Println()
	fmt.Println("type \"help\" for available commands")
	fmt.Println()

	channels := make(map[string]string)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("angelcord> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := splitArgs(line)
		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "help":
			printHelp()
		case "whoami":
			cmdWhoami(sess)
		case "users":
			cmdUsers(sess)
		case "guilds":
			cmdGuilds(sess)
		case "guild":
			cmdGuild(sess, args)
		case "create-guild":
			cmdCreateGuild(sess, args)
		case "create-channel":
			cmdCreateChannel(channels, args)
		case "channels":
			cmdChannels(channels)
		case "members":
			cmdMembers(sess, args)
		case "invite":
			cmdInvite(sess, args)
		case "kick":
			cmdKick(sess, args)
		case "sync":
			cmdSync(sess, args)
		case "send":
			cmdSend(sess, args, channels)
		case "read":
			cmdRead(sess, args, channels)
		case "exit", "quit":
			fmt.Println("bye")
			return
		default:
			fmt.Printf("unknown command: %s (type \"help\")\n", cmd)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		fmt.Fprintf(os.Stderr, "input error: %v\n", err)
	}
}

func splitArgs(s string) []string {
	var parts []string
	var current strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func printHelp() {
	fmt.Println(`commands:
  whoami                              show your identity + public key
  users                               list all registered users
  guilds                              list your guilds
  guild GUILD_ID                      show guild details
  create-guild "NAME"                 create a new guild (you become owner)
  create-channel NAME                 create a client-local channel (UUID)
  channels                            list client-local channels
  members GUILD_ID                    list guild members (pubkeys)
  invite GUILD_ID PUBKEY              invite a user by their public key
  kick GUILD_ID PUBKEY                kick a member
  sync GUILD_ID                       download + decrypt guild key
  send GUILD_ID CHANNEL message...    encrypt + send a message
  read GUILD_ID [CHANNEL]             fetch + decrypt messages
  exit                                quit`)
}

func cmdWhoami(sess *Session) {
	fmt.Printf("  name:       %s\n", sess.User.Name)
	fmt.Printf("  public key: %s\n", sess.PubKeyB64())
}

func cmdUsers(sess *Session) {
	users, err := sess.HTTP.ListUsers()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	if len(users) == 0 {
		fmt.Println("  (no users)")
		return
	}
	for _, u := range users {
		tag := ""
		if u.PubKey == sess.PubKeyB64() {
			tag = " (you)"
		}
		fmt.Printf("  %s  %s%s\n", u.PubKey[:16]+"...", u.Name, tag)
	}
}

func cmdGuilds(sess *Session) {
	gs, err := sess.HTTP.ListGuilds()
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	if len(gs) == 0 {
		fmt.Println("  (no guilds)")
		return
	}
	for _, g := range gs {
		fmt.Printf("  %-36s  %s  (%d members)\n",
			g.ID, g.Name, len(g.Members))
	}
}

func cmdGuild(sess *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("  usage: guild GUILD_ID")
		return
	}
	g, err := sess.HTTP.GetGuild(args[0])
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Printf("  id:      %s\n", g.ID)
	fmt.Printf("  name:    %s\n", g.Name)
	fmt.Printf("  owner:   %s\n", g.OwnerPub[:16]+"...")
	fmt.Printf("  members: %d\n", len(g.Members))
	for _, pk := range g.Members {
		tag := ""
		if pk == sess.PubKeyB64() {
			tag = " (you)"
		}
		fmt.Printf("    - %s%s\n", pk[:16]+"...", tag)
	}
}

func cmdCreateGuild(sess *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("  usage: create-guild \"NAME\"")
		return
	}
	name := strings.Join(args, " ")
	g, err := sess.CreateGuild(name)
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Printf("  created guild %s (%s)\n", g.Name, g.ID)
	fmt.Printf("  guild key generated and uploaded\n")
}

func cmdCreateChannel(channels map[string]string, args []string) {
	if len(args) < 1 {
		fmt.Println("  usage: create-channel NAME")
		return
	}
	name := args[0]
	id := uuid.New().String()
	channels[name] = id
	fmt.Printf("  created channel #%s (%s)\n", name, id)
	fmt.Println("  (client-local only -- share the name with guild members out-of-band)")
}

func cmdChannels(channels map[string]string) {
	if len(channels) == 0 {
		fmt.Println("  (no channels)")
		return
	}
	for name, id := range channels {
		fmt.Printf("  #%-20s %s\n", name, id)
	}
}

func cmdMembers(sess *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("  usage: members GUILD_ID")
		return
	}
	members, err := sess.HTTP.ListMembers(args[0])
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	if len(members) == 0 {
		fmt.Println("  (no members)")
		return
	}
	for _, pk := range members {
		tag := ""
		if pk == sess.PubKeyB64() {
			tag = " (you)"
		}
		fmt.Printf("  %s%s\n", pk[:16]+"...", tag)
	}
}

func cmdInvite(sess *Session, args []string) {
	if len(args) < 2 {
		fmt.Println("  usage: invite GUILD_ID PUBKEY")
		return
	}
	if err := sess.InviteMember(args[0], args[1]); err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Printf("  invited %s...\n", args[1][:16])
}

func cmdKick(sess *Session, args []string) {
	if len(args) < 2 {
		fmt.Println("  usage: kick GUILD_ID PUBKEY")
		return
	}
	if err := sess.KickMember(args[0], args[1]); err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Printf("  kicked %s...\n", args[1][:16])
}

func cmdSync(sess *Session, args []string) {
	if len(args) < 1 {
		fmt.Println("  usage: sync GUILD_ID")
		return
	}
	if err := sess.SyncGuildKey(args[0]); err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Println("  guild key synced")
}

func cmdSend(sess *Session, args []string, channels map[string]string) {
	if len(args) < 3 {
		fmt.Println("  usage: send GUILD_ID CHANNEL message text...")
		return
	}
	guildID := args[0]
	channelArg := args[1]
	content := strings.Join(args[2:], " ")

	channelID := channelArg
	if id, ok := channels[channelArg]; ok {
		channelID = id
	}

	if err := sess.SendMessage(guildID, channelID, content); err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	fmt.Println("  sent (encrypted + signed)")
}

func cmdRead(sess *Session, args []string, channels map[string]string) {
	if len(args) < 1 {
		fmt.Println("  usage: read GUILD_ID [CHANNEL]")
		return
	}
	guildID := args[0]
	channelID := ""
	if len(args) >= 2 {
		channelArg := args[1]
		channelID = channelArg
		if id, ok := channels[channelArg]; ok {
			channelID = id
		}
	}

	msgs, err := sess.ReadMessages(guildID, channelID)
	if err != nil {
		fmt.Printf("  error: %v\n", err)
		return
	}
	if len(msgs) == 0 {
		fmt.Println("  (no messages)")
		return
	}
	for _, m := range msgs {
		ts := m.Timestamp.Format("15:04:05")
		sender := m.SenderPub
		if len(sender) > 16 {
			sender = sender[:16] + "..."
		}
		if m.SenderPub == sess.PubKeyB64() {
			sender = "you"
		}
		if m.Error != "" {
			fmt.Printf("  [%s] %s: <%s>\n", ts, sender, m.Error)
		} else {
			fmt.Printf("  [%s] %s: %s\n", ts, sender, m.Content)
		}
	}
}

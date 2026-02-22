package angelcord_test

import (
	"net/http/httptest"
	"testing"

	"github.com/anthropic/angelcord/internal/client"
	"github.com/anthropic/angelcord/internal/guild"
	"github.com/anthropic/angelcord/internal/server"
)

func startServer(t *testing.T) *httptest.Server {
	t.Helper()
	gs := guild.NewMemoryGuildState()
	relay := server.NewRelayStore()
	router := server.NewRouter(gs, relay)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)
	return ts
}

func newUser(t *testing.T, name, serverURL string) *client.Session {
	t.Helper()
	sess, err := client.NewSession(name, serverURL)
	if err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
	t.Logf("%s pubkey = %s...", name, sess.PubKeyB64()[:16])
	return sess
}

// TestE2E_BasicMessaging tests: guild creation, messaging between two members,
// channel filtering, and guild key distribution.
func TestE2E_BasicMessaging(t *testing.T) {
	ts := startServer(t)

	alice := newUser(t, "alice", ts.URL)
	bob := newUser(t, "bob", ts.URL)

	g, err := alice.CreateGuild("test-guild")
	if err != nil {
		t.Fatalf("create guild: %v", err)
	}
	t.Logf("guild = %s", g.ID)

	if err := alice.InviteMember(g.ID, bob.PubKeyB64()); err != nil {
		t.Fatalf("invite bob: %v", err)
	}

	if err := bob.SyncGuildKey(g.ID); err != nil {
		t.Fatalf("bob sync: %v", err)
	}

	generalCh := "channel-general"
	randomCh := "channel-random"

	if err := alice.SendMessage(g.ID, generalCh, "hello from alice!"); err != nil {
		t.Fatalf("alice send: %v", err)
	}
	if err := bob.SendMessage(g.ID, generalCh, "hello from bob!"); err != nil {
		t.Fatalf("bob send: %v", err)
	}
	if err := alice.SendMessage(g.ID, randomCh, "random stuff"); err != nil {
		t.Fatalf("alice send random: %v", err)
	}

	msgs, err := bob.ReadMessages(g.ID, generalCh)
	if err != nil {
		t.Fatalf("bob read general: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages in #general, got %d", len(msgs))
	}
	for _, m := range msgs {
		if m.Error != "" {
			t.Errorf("message error: %s", m.Error)
		}
	}
	if msgs[0].Content != "hello from alice!" {
		t.Errorf("msg[0] = %q, want %q", msgs[0].Content, "hello from alice!")
	}
	if msgs[1].Content != "hello from bob!" {
		t.Errorf("msg[1] = %q, want %q", msgs[1].Content, "hello from bob!")
	}

	rmsgs, err := bob.ReadMessages(g.ID, randomCh)
	if err != nil {
		t.Fatalf("bob read random: %v", err)
	}
	if len(rmsgs) != 1 || rmsgs[0].Content != "random stuff" {
		t.Fatalf("unexpected random messages: %+v", rmsgs)
	}

	all, err := alice.ReadMessages(g.ID, "")
	if err != nil {
		t.Fatalf("alice read all: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 total messages, got %d", len(all))
	}

	t.Log("=== basic messaging PASS ===")
}

// TestE2E_InviteReadsHistory tests that a new member can decrypt all
// historical messages after receiving the guild key.
func TestE2E_InviteReadsHistory(t *testing.T) {
	ts := startServer(t)

	alice := newUser(t, "alice", ts.URL)
	bob := newUser(t, "bob", ts.URL)
	carol := newUser(t, "carol", ts.URL)

	g, err := alice.CreateGuild("history-guild")
	if err != nil {
		t.Fatalf("create guild: %v", err)
	}

	if err := alice.InviteMember(g.ID, bob.PubKeyB64()); err != nil {
		t.Fatalf("invite bob: %v", err)
	}
	if err := bob.SyncGuildKey(g.ID); err != nil {
		t.Fatalf("bob sync: %v", err)
	}

	ch := "general"

	if err := alice.SendMessage(g.ID, ch, "msg1"); err != nil {
		t.Fatalf("alice send: %v", err)
	}
	if err := bob.SendMessage(g.ID, ch, "msg2"); err != nil {
		t.Fatalf("bob send: %v", err)
	}
	if err := alice.SendMessage(g.ID, ch, "msg3"); err != nil {
		t.Fatalf("alice send: %v", err)
	}

	if err := alice.InviteMember(g.ID, carol.PubKeyB64()); err != nil {
		t.Fatalf("invite carol: %v", err)
	}
	if err := carol.SyncGuildKey(g.ID); err != nil {
		t.Fatalf("carol sync: %v", err)
	}

	msgs, err := carol.ReadMessages(g.ID, ch)
	if err != nil {
		t.Fatalf("carol read: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("carol expected 3 messages, got %d", len(msgs))
	}
	for i, m := range msgs {
		if m.Error != "" {
			t.Errorf("carol msg[%d] error: %s", i, m.Error)
		}
	}
	if msgs[0].Content != "msg1" || msgs[1].Content != "msg2" || msgs[2].Content != "msg3" {
		t.Errorf("unexpected message contents: %v %v %v", msgs[0].Content, msgs[1].Content, msgs[2].Content)
	}

	t.Log("=== invite reads history PASS ===")
}

// TestE2E_KickAndRevocation tests that after Eve is kicked, she can no longer
// read or write messages because the server enforces membership.
func TestE2E_KickAndRevocation(t *testing.T) {
	ts := startServer(t)

	alice := newUser(t, "alice", ts.URL)
	bob := newUser(t, "bob", ts.URL)
	eve := newUser(t, "eve", ts.URL)

	g, err := alice.CreateGuild("kick-guild")
	if err != nil {
		t.Fatalf("create guild: %v", err)
	}

	if err := alice.InviteMember(g.ID, bob.PubKeyB64()); err != nil {
		t.Fatalf("invite bob: %v", err)
	}
	if err := alice.InviteMember(g.ID, eve.PubKeyB64()); err != nil {
		t.Fatalf("invite eve: %v", err)
	}
	if err := bob.SyncGuildKey(g.ID); err != nil {
		t.Fatalf("bob sync: %v", err)
	}
	if err := eve.SyncGuildKey(g.ID); err != nil {
		t.Fatalf("eve sync: %v", err)
	}

	ch := "general"

	if err := alice.SendMessage(g.ID, ch, "before-kick"); err != nil {
		t.Fatalf("alice send pre-kick: %v", err)
	}

	preMsgs, err := eve.ReadMessages(g.ID, ch)
	if err != nil {
		t.Fatalf("eve read pre-kick: %v", err)
	}
	if len(preMsgs) != 1 || preMsgs[0].Content != "before-kick" {
		t.Fatalf("eve pre-kick read unexpected: %+v", preMsgs)
	}

	if err := alice.KickMember(g.ID, eve.PubKeyB64()); err != nil {
		t.Fatalf("kick eve: %v", err)
	}

	if err := alice.SendMessage(g.ID, ch, "after-kick"); err != nil {
		t.Fatalf("alice send post-kick: %v", err)
	}

	// Bob can still read both messages.
	bobMsgs, err := bob.ReadMessages(g.ID, ch)
	if err != nil {
		t.Fatalf("bob read post-kick: %v", err)
	}
	if len(bobMsgs) != 2 {
		t.Fatalf("bob expected 2 messages, got %d", len(bobMsgs))
	}
	if bobMsgs[0].Content != "before-kick" || bobMsgs[1].Content != "after-kick" {
		t.Errorf("bob messages: %q, %q", bobMsgs[0].Content, bobMsgs[1].Content)
	}

	// Eve is blocked by the server -- she can't fetch messages anymore.
	_, err = eve.ReadMessages(g.ID, ch)
	if err == nil {
		t.Fatal("expected eve to be blocked from reading after kick, but got no error")
	}
	t.Logf("eve correctly blocked: %v", err)

	// Eve can't send messages either.
	err = eve.SendMessage(g.ID, ch, "sneaky message")
	if err == nil {
		t.Fatal("expected eve to be blocked from sending after kick, but got no error")
	}
	t.Logf("eve correctly blocked from sending: %v", err)

	t.Log("=== kick and revocation PASS ===")
}

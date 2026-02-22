# hespercord (angelcord)

Because Discord should be a commodity, and messages should be encrypted on the
server.

> [!INFO]
> This README is AI-written for now, but gives a decent overview. I will
> rewrite it soon.

A Discord-style chat app prototype where the server never sees plaintext.
Guilds (servers) and channels exist, but message content and channel names are
encrypted end-to-end. DMs use the full Signal Protocol; guilds use a single
shared key per server with a few extra integrity checks. The goal is something
that feels like early Discord but keeps the relay as dumb and commodity as
possible—you could swap the backend provider freely.

## Architecture

**Server.** A single Go program: REST API, WebSockets for live updates,
optional *Discord OAuth* for the web app. It stores ciphertext blobs, sealed
key bundles, and user/session data. It does *not* do encryption or decryption.
Guild *membership* and metadata (name, owner, who’s in) are behind a
`GuildState` *interface*—today that’s SQLite, with the idea that later it could
be a Solana *program* or similar so the relay stays a simple message bus.

**Clients.** Two: a Go CLI (for testing and API-level use) and a React/Vite web
app. Both do all crypto locally. The server only sees base64 ciphertext and
*routing info* (guild id, sender/recipient public keys).

**Guilds and channels.** One guild = one symmetric key. Channels are not
*first-class* on the server: the client encrypts a payload that includes
channel_id, content, a sequence number, and a signature. The server stores one
opaque blob per message; channel list and names live only on the client (or, in
a future design, on-chain). The server *does* assign and store a timestamp per
message and returns messages ordered by that time—it just never sees the
content or channel. New members get the guild key sealed under their X25519
public key by an existing member (usually the inviter); when someone is kicked,
the server stops letting them read/write and they no longer get new sealed
keys—we don’t rotate the guild key *on kick*, so the model is “one permanent
key, revocation = access control.”

**DMs.** One-to-one messages use the real Signal Protocol: X3DH for session
setup, the Double Ratchet for encryption, and PQXDH (Kyber) in the pre-key
bundle. The web app uses `@getmaapp/signal-wasm` (libsignal compiled to WASM).
Session and pre-key state live in IndexedDB. The Signal identity is derived
from the same Ed25519/X25519 app identity so one login and one key backup
covers both guild auth and DM identity.

## Crypto choices

**Identity.** Ed25519 for “who you are”: API auth (CLI signs requests, web uses
*JWT* that carries your public key), and signing the inner guild message
payload. X25519 is derived from the same keypair (standard Ed25519→Curve25519
mapping) and used for ECDH when sealing guild keys and, in the web client, as
the Signal *long-term identity*. So there’s a single root keypair; everything
else branches from it.

**Guild messages.** One 32-byte key per guild. Messages are encrypted with
*XChaCha20-Poly1305*. The plaintext the client encrypts is a small JSON object:
channel id, body, a per-sender sequence number, and an Ed25519 signature over
that triple. Receivers verify the signature and enforce that sequence numbers
only go forward for each sender—that gives replay and reorder detection without
a full ratchet. The server never sees plaintext or channel names.

**Guild key distribution.** On guild creation the creator generates the 32-byte
guild key client-side. The first sealed key is for *themselves*: they run ECDH
with their own X25519 keypair (their private key and their own public key as
“recipient”), derive a symmetric key from that shared secret (SHA-256), and
encrypt the guild key with XChaCha20-Poly1305. They upload that blob to the
server with themselves as the recipient—so the server holds a copy only they
can unseal (e.g. when opening the guild on another device). For invites it’s
the same process: ECDH (inviter’s private key, invitee’s public key), then
encrypt the guild key and upload with the invitee as recipient. The server
stores these sealed blobs and hands them to the right recipient when they ask
for their keys. No plaintext guild key on the server.

**DMs.** Full Signal: X3DH (with an optional one-time pre-key and a signed
*pre-key*, plus Kyber) so the first message establishes a session without an
interactive handshake; then the Double Ratchet for forward secrecy and
post-compromise security on the wire. We’re not doing anything custom here—we
rely on libsignal’s design and the WASM build that wraps it.

## How this lines up with other designs

The Double Ratchet (e.g. Signal’s spec, “Double Ratchet Algorithm” v2/v3/v4) is
built for two parties and forward secrecy: each message can advance the chain,
and you don’t want the server or a future compromise to decrypt old traffic. We
use that exactly for DMs. For guilds we intentionally don’t: we want a shared
key so that history is readable by everyone in the guild and new members can
decrypt past messages once they get the key. So guild crypto is “one key, good
AEAD, and integrity checks” rather than ratcheting. The per-sender sequence
number and signature are a minimal way to catch dropped, reordered, or replayed
messages without the complexity of a ratchet or per-sender chains. Signal’s
group story (sender keys, distribution messages) is more than we need for this
prototype; we traded that for simplicity and the ability to hand one key to new
members.

<!-- Session Protocol gives you E2EE over a different topology—discrete nodes, onion-style routing, no single server. Their closed groups have their own key and membership rules. We’re not trying to be serverless or anonymous; we have one relay and optional Discord SSO. The part we borrowed conceptually is “one strong identity (curve keys), use it for both auth and E2EE,” and for DMs we literally use the same protocol family (X3DH + Double Ratchet) that Signal and Session-style stacks use. Where Session goes further is in routing and metadata hiding; we’re at “encrypt the payload, server sees ciphertext and who’s talking to whom.” -->

So: guilds are deliberately simple (one key, no ratchet, revocation by
membership); DMs are full Signal so they get the same guarantees as modern
Signal 1:1; and the server stays a relay that could be swapped or complemented
by an on-chain layer for guild state.

## Repo layout

- `cmd/server` — Hespercord server (API, WS, OAuth, SPA serving).
- `cmd/client` — CLI client (Go).
- `internal/crypto` — Key generation, sealing, message encryption, signing (guild side).
- `internal/guild` — `GuildState` interface + SQLite (and in-memory for tests).
- `internal/protocol` — Wire types (JSON) for API and messages.
- `internal/server` — HTTP handlers, DB, WebSocket hub, auth, OAuth.
- `web/` — React app: guild list, channels, DMs, Signal WASM integration, key backup.

## Running

**Server (with SQLite and web):**
```bash
go run ./cmd/server -addr :8080 -db hespercord.db -web web/dist
```

Build the web app first: `cd web && npm install && npm run build`. For Discord
login, set `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`, and
`DISCORD_REDIRECT_URL`.

**CLI client:**  
`go run ./cmd/angelcord` (or the client binary) with a server URL; it will
prompt for name and use Ed25519-signed requests.

The server stores only ciphertext and key bundles; guild membership and metadata
are in the configured `GuildState` backend (SQLite by default).

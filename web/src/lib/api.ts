const BASE = ''

async function fetchJSON<T>(path: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(BASE + path, {
    ...opts,
    headers: { 'Content-Type': 'application/json', ...opts?.headers },
    credentials: 'include',
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || `HTTP ${res.status}`)
  }
  return res.json()
}

// --- Auth ---

export interface AuthMe {
  discord_id: string
  username: string
  avatar_url?: string
  is_new: boolean
  ed25519_pub?: string
  x25519_pub?: string
  encrypted_privkey?: string
  key_salt?: string
  key_iterations?: number
}

export function getMe(): Promise<AuthMe> {
  return fetchJSON('/auth/me')
}

export function signup(data: {
  ed25519_pub: string
  x25519_pub: string
  encrypted_privkey: string
  key_salt: string
  key_iterations: number
}): Promise<{ status: string }> {
  return fetchJSON('/auth/signup', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function logout(): Promise<void> {
  return fetchJSON('/auth/logout', { method: 'POST' })
}

// --- Users ---

export interface User {
  name: string
  pub_key: string
}

export function listUsers(): Promise<User[]> {
  return fetchJSON('/api/users')
}

// --- Guilds ---

export interface Guild {
  id: string
  name: string
  owner_pub: string
  members: string[]
}

export function createGuild(name: string): Promise<Guild> {
  return fetchJSON('/api/guilds', {
    method: 'POST',
    body: JSON.stringify({ name }),
  })
}

export function listGuilds(): Promise<Guild[]> {
  return fetchJSON('/api/guilds')
}

export function getGuild(id: string): Promise<Guild> {
  return fetchJSON(`/api/guilds/${id}`)
}

// --- Members ---

export function inviteMember(
  guildId: string,
  inviteePubKey: string,
  sealedKeys: { sealed_key_b64: string; sealer_pub: string }[]
): Promise<{ status: string }> {
  return fetchJSON(`/api/guilds/${guildId}/invite`, {
    method: 'POST',
    body: JSON.stringify({
      invitee_pub_key: inviteePubKey,
      sealed_keys: sealedKeys,
    }),
  })
}

export function kickMember(guildId: string, pubKey: string): Promise<{ status: string }> {
  const urlSafe = pubKey.replace(/\+/g, '-').replace(/\//g, '_')
  return fetchJSON(`/api/guilds/${guildId}/members/${urlSafe}`, {
    method: 'DELETE',
  })
}

export function listMembers(guildId: string): Promise<string[]> {
  return fetchJSON(`/api/guilds/${guildId}/members`)
}

// --- Keys ---

export interface KeyBundle {
  guild_id: string
  recipient_pub: string
  keys: { sealed_key_b64: string; sealer_pub: string }[]
}

export function uploadKeys(
  guildId: string,
  bundles: KeyBundle[]
): Promise<{ status: string }> {
  return fetchJSON(`/api/guilds/${guildId}/keys`, {
    method: 'POST',
    body: JSON.stringify({ bundles }),
  })
}

export function getKeys(guildId: string): Promise<KeyBundle> {
  return fetchJSON(`/api/guilds/${guildId}/keys`)
}

export function uploadGuildAvatar(guildId: string, encryptedAvatarB64: string): Promise<{ status: string }> {
  return fetchJSON(`/api/guilds/${guildId}/avatars`, {
    method: 'POST',
    body: JSON.stringify({ encrypted_avatar_b64: encryptedAvatarB64 }),
  })
}

export interface GuildMemberAvatar {
  member_pub: string
  encrypted_avatar_b64: string
}

export function getGuildAvatars(guildId: string): Promise<{ avatars: GuildMemberAvatar[] }> {
  return fetchJSON(`/api/guilds/${guildId}/avatars`)
}

// --- Messages ---

export interface MessageEnvelope {
  id: string
  guild_id: string
  sender_pub: string
  ciphertext_b64: string
  timestamp: string
}

export function sendMessage(
  guildId: string,
  ciphertextB64: string
): Promise<MessageEnvelope> {
  return fetchJSON(`/api/guilds/${guildId}/messages`, {
    method: 'POST',
    body: JSON.stringify({ ciphertext_b64: ciphertextB64 }),
  })
}

export function getMessages(
  guildId: string,
  after?: string
): Promise<MessageEnvelope[]> {
  let path = `/api/guilds/${guildId}/messages`
  if (after) path += `?after=${encodeURIComponent(after)}`
  return fetchJSON(path)
}

// --- DMs ---

export interface DMMessage {
  id: string
  sender_pub: string
  recipient_pub: string
  ciphertext_b64: string
  message_type: number
  timestamp: string
}

export function sendDM(
  recipientPub: string,
  ciphertextB64: string,
  messageType: number
): Promise<DMMessage> {
  const urlSafe = recipientPub.replace(/\+/g, '-').replace(/\//g, '_')
  return fetchJSON(`/api/dms/${urlSafe}/messages`, {
    method: 'POST',
    body: JSON.stringify({ ciphertext_b64: ciphertextB64, message_type: messageType }),
  })
}

export function getDMs(
  peerPub: string,
  after?: string
): Promise<DMMessage[]> {
  const urlSafe = peerPub.replace(/\+/g, '-').replace(/\//g, '_')
  let path = `/api/dms/${urlSafe}/messages`
  if (after) path += `?after=${encodeURIComponent(after)}`
  return fetchJSON(path)
}

export function listDMConversations(): Promise<string[]> {
  return fetchJSON('/api/dms')
}

// --- Signal Pre-Keys ---

export interface PreKeyBundleResponse {
  pub_key: string
  registration_id: number
  identity_key_pub: string
  signed_prekey_id: number
  signed_prekey_pub: string
  signed_prekey_sig: string
  one_time_prekey?: { prekey_id: number; prekey_pub: string }
  kyber_prekey_id?: number
  kyber_prekey_pub?: string
  kyber_prekey_sig?: string
}

export function uploadPreKeys(data: {
  pub_key: string
  registration_id: number
  identity_key_pub: string
  signed_prekey_id: number
  signed_prekey_pub: string
  signed_prekey_sig: string
  one_time_prekeys: { prekey_id: number; prekey_pub: string }[]
  kyber_prekey_id?: number
  kyber_prekey_pub?: string
  kyber_prekey_sig?: string
}): Promise<{ status: string }> {
  return fetchJSON('/api/signal/prekeys', {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function getPreKeys(pubKey: string): Promise<PreKeyBundleResponse> {
  const urlSafe = pubKey.replace(/\+/g, '-').replace(/\//g, '_')
  return fetchJSON(`/api/signal/prekeys/${urlSafe}`)
}

import { create } from 'zustand'
import * as api from '../lib/api'
import {
  generateGuildKey,
  sealKey,
  unsealKey,
  encryptMessage,
  decryptMessage,
  edPublicToX25519,
  toBase64,
  fromBase64,
} from '../lib/crypto'
import { sha256 } from '@noble/hashes/sha2.js'
import { useAuthStore } from './auth'
import { wsClient } from '../lib/ws'

function channelId(guildId: string, name: string): string {
  const hash = sha256(new TextEncoder().encode(`${guildId}:${name}`))
  const hex = Array.from(hash.slice(0, 16), b => b.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0,8)}-${hex.slice(8,12)}-${hex.slice(12,16)}-${hex.slice(16,20)}-${hex.slice(20,32)}`
}

interface Channel {
  id: string
  name: string
}

interface GuildState {
  guilds: api.Guild[]
  selectedGuildId: string | null
  selectedChannelId: string | null
  channels: Record<string, Channel[]> // guildId -> channels (client-local)
  guildKeys: Record<string, Uint8Array> // guildId -> symmetric key
  avatarUrlsByGuild: Record<string, Record<string, string>> // guildId -> memberPub -> avatar URL

  loadGuilds: () => Promise<void>
  selectGuild: (id: string) => void
  selectChannel: (id: string) => void
  createGuild: (name: string) => Promise<api.Guild>
  createChannel: (guildId: string, name: string) => void
  inviteMember: (guildId: string, pubKey: string) => Promise<void>
  kickMember: (guildId: string, pubKey: string) => Promise<void>
  syncGuildKey: (guildId: string) => Promise<void>
  loadGuildAvatars: (guildId: string) => Promise<void>
  getGuildKey: (guildId: string) => Uint8Array | null
  getAvatarUrl: (guildId: string, memberPub: string) => string | null
}

export const useGuildStore = create<GuildState>((set, get) => ({
  guilds: [],
  selectedGuildId: null,
  selectedChannelId: null,
  channels: {},
  guildKeys: {},
  avatarUrlsByGuild: {},

  loadGuilds: async () => {
    try {
      const guilds = await api.listGuilds()
      set({ guilds: guilds || [] })
      for (const g of (guilds || [])) {
        wsClient.subscribe(g.id)
      }
    } catch {
      set({ guilds: [] })
    }
  },

  selectGuild: (id: string) => {
    set({ selectedGuildId: id })
    const channels = get().channels[id]
    if (channels?.length) {
      set({ selectedChannelId: channels[0].id })
    } else {
      const generalId = channelId(id, 'general')
      set({
        selectedChannelId: generalId,
        channels: {
          ...get().channels,
          [id]: [{ id: generalId, name: 'general' }],
        },
      })
    }
    // Sync guild key in background
    get().syncGuildKey(id)
  },

  selectChannel: (id: string) => set({ selectedChannelId: id }),

  createGuild: async (name: string) => {
    const identity = useAuthStore.getState().identity
    if (!identity) throw new Error('Not authenticated')

    const guild = await api.createGuild(name)

    const guildKey = generateGuildKey()
    const sealed = sealKey(guildKey, identity.xPriv, identity.xPub)
    await api.uploadKeys(guild.id, [{
      guild_id: guild.id,
      recipient_pub: toBase64(identity.edPub),
      keys: [{ sealed_key_b64: toBase64(sealed), sealer_pub: toBase64(identity.edPub) }],
    }])

    const generalId = channelId(guild.id, 'general')
    wsClient.subscribe(guild.id)
    set({
      guilds: [...get().guilds, guild],
      guildKeys: { ...get().guildKeys, [guild.id]: guildKey },
      channels: {
        ...get().channels,
        [guild.id]: [{ id: generalId, name: 'general' }],
      },
    })
    return guild
  },

  createChannel: (guildId: string, name: string) => {
    const id = channelId(guildId, name)
    const existing = get().channels[guildId] || []
    set({
      channels: {
        ...get().channels,
        [guildId]: [...existing, { id, name }],
      },
    })
  },

  inviteMember: async (guildId: string, pubKey: string) => {
    const identity = useAuthStore.getState().identity
    if (!identity) throw new Error('Not authenticated')

    const guildKey = get().guildKeys[guildId]
    if (!guildKey) throw new Error('No guild key')

    const inviteeXPub = edPublicToX25519(fromBase64(pubKey))
    const sealed = sealKey(guildKey, identity.xPriv, inviteeXPub)

    await api.inviteMember(guildId, pubKey, [
      { sealed_key_b64: toBase64(sealed), sealer_pub: toBase64(identity.edPub) },
    ])
  },

  kickMember: async (guildId: string, pubKey: string) => {
    await api.kickMember(guildId, pubKey)
  },

  syncGuildKey: async (guildId: string) => {
    const identity = useAuthStore.getState().identity
    if (!identity) return

    const alreadyHadKey = !!get().guildKeys[guildId]
    if (!alreadyHadKey) {
      try {
        const bundle = await api.getKeys(guildId)
        if (!bundle.keys?.length) return

        const entry = bundle.keys[0]
        const sealerXPub = edPublicToX25519(fromBase64(entry.sealer_pub))
        const sealedBytes = fromBase64(entry.sealed_key_b64)
        const guildKey = unsealKey(sealedBytes, identity.xPriv, sealerXPub)

        set({ guildKeys: { ...get().guildKeys, [guildId]: guildKey } })
      } catch { /* key not available yet */ return }
    }

    const guildKey = get().guildKeys[guildId]
    if (!guildKey) return

    const avatarUrl = useAuthStore.getState().avatarUrl
    if (avatarUrl) {
      try {
        const encrypted = encryptMessage(guildKey, new TextEncoder().encode(avatarUrl))
        await api.uploadGuildAvatar(guildId, toBase64(encrypted))
      } catch { /* ignore */ }
    }

    get().loadGuildAvatars(guildId)
  },

  loadGuildAvatars: async (guildId: string) => {
    const guildKey = get().guildKeys[guildId]
    if (!guildKey) return

    try {
      const { avatars } = await api.getGuildAvatars(guildId)
      const byMember: Record<string, string> = {}
      for (const a of avatars || []) {
        try {
          const sealed = fromBase64(a.encrypted_avatar_b64)
          const plain = decryptMessage(guildKey, sealed)
          byMember[a.member_pub] = new TextDecoder().decode(plain)
        } catch { /* skip invalid */ }
      }
      set({
        avatarUrlsByGuild: {
          ...get().avatarUrlsByGuild,
          [guildId]: { ...get().avatarUrlsByGuild[guildId], ...byMember },
        },
      })
    } catch { /* ignore */ }
  },

  getGuildKey: (guildId: string) => get().guildKeys[guildId] || null,

  getAvatarUrl: (guildId: string, memberPub: string) => {
    const byMember = get().avatarUrlsByGuild[guildId]
    return byMember?.[memberPub] ?? null
  },
}))

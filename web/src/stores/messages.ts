import { create } from 'zustand'
import * as api from '../lib/api'
import {
  encryptMessage,
  decryptMessage,
  signMessagePayload,
  verifySignature,
  toBase64,
  fromBase64,
} from '../lib/crypto'
import { useAuthStore } from './auth'
import { useGuildStore } from './guilds'

export interface DecryptedMessage {
  id: string
  senderPub: string
  channelId: string
  content: string
  seq: number
  timestamp: string
  error?: string
}

interface MessageState {
  messages: Record<string, DecryptedMessage[]> // guildId:channelId -> messages
  seqCounters: Record<string, number> // guildId -> next seq

  loadMessages: (guildId: string, channelId: string) => Promise<void>
  sendMessage: (guildId: string, channelId: string, content: string) => Promise<void>
  addMessage: (guildId: string, envelope: api.MessageEnvelope) => void
}

export const useMessageStore = create<MessageState>((set, get) => ({
  messages: {},
  seqCounters: {},

  loadMessages: async (guildId: string, channelId: string) => {
    const envelopes = await api.getMessages(guildId)
    if (!envelopes) return
    const key = `${guildId}:${channelId}`
    const decrypted: DecryptedMessage[] = []

    for (const env of envelopes) {
      const msg = decryptEnvelope(env)
      if (msg.channelId === channelId || !channelId) {
        decrypted.push(msg)
      }
    }

    set({ messages: { ...get().messages, [key]: decrypted } })
  },

  sendMessage: async (guildId: string, channelId: string, content: string) => {
    const identity = useAuthStore.getState().identity
    if (!identity) throw new Error('Not authenticated')

    await useGuildStore.getState().syncGuildKey(guildId)
    const guildKey = useGuildStore.getState().guildKeys[guildId]
    if (!guildKey) throw new Error('No guild key')

    const seq = get().seqCounters[guildId] || 0
    set({ seqCounters: { ...get().seqCounters, [guildId]: seq + 1 } })

    const sig = signMessagePayload(identity.edPriv, channelId, content, seq)
    const inner = JSON.stringify({
      channel_id: channelId,
      content,
      seq,
      sig: toBase64(sig),
    })

    const ciphertext = encryptMessage(guildKey, new TextEncoder().encode(inner))
    const env = await api.sendMessage(guildId, toBase64(ciphertext))

    const decrypted: DecryptedMessage = {
      id: env.id,
      senderPub: toBase64(identity.edPub),
      channelId,
      content,
      seq,
      timestamp: env.timestamp,
    }

    const key = `${guildId}:${channelId}`
    const existing = get().messages[key] || []
    if (!existing.find(m => m.id === decrypted.id)) {
      set({ messages: { ...get().messages, [key]: [...existing, decrypted] } })
    }
  },

  addMessage: (guildId: string, envelope: api.MessageEnvelope) => {
    const msg = decryptEnvelope(envelope)
    // Add to all matching channel keys
    const state = get()
    const updated = { ...state.messages }
    const key = `${guildId}:${msg.channelId}`
    const existing = updated[key] || []
    if (!existing.find(m => m.id === msg.id)) {
      updated[key] = [...existing, msg]
      set({ messages: updated })
    }
  },
}))

function decryptEnvelope(env: api.MessageEnvelope): DecryptedMessage {
  const base: DecryptedMessage = {
    id: env.id,
    senderPub: env.sender_pub,
    channelId: '',
    content: '',
    seq: 0,
    timestamp: env.timestamp,
  }

  const guildKey = useGuildStore.getState().guildKeys[env.guild_id]
  if (!guildKey) {
    return { ...base, error: 'missing guild key' }
  }

  try {
    const ctBytes = fromBase64(env.ciphertext_b64)
    const plaintext = decryptMessage(guildKey, ctBytes)
    const inner = JSON.parse(new TextDecoder().decode(plaintext))

    const senderPub = fromBase64(env.sender_pub)
    const sigBytes = fromBase64(inner.sig)
    if (!verifySignature(senderPub, inner.channel_id, inner.content, inner.seq, sigBytes)) {
      return { ...base, error: 'signature verification failed' }
    }

    return {
      ...base,
      channelId: inner.channel_id,
      content: inner.content,
      seq: inner.seq,
    }
  } catch (e: any) {
    return { ...base, error: e.message }
  }
}

import { create } from 'zustand'
import * as api from '../lib/api'
import { signalManager } from '../lib/signal'
import { cachePut, cacheGetMany, type CachedMessage } from '../lib/dm-cache'

/** A DM message with its decrypted plaintext (or a placeholder if undecryptable). */
export interface ResolvedDM {
  id: string
  senderPub: string
  recipientPub: string
  timestamp: string
  plaintext: string
}

export interface DMConversation {
  peerPub: string
  peerName?: string
}

interface DMState {
  conversations: DMConversation[]
  selectedPeer: string | null
  messages: Record<string, ResolvedDM[]>
  _myPub: string

  setMyPub: (pub: string) => void
  loadConversations: () => Promise<void>
  selectPeer: (pub: string) => void
  loadDMHistory: (peerPub: string) => Promise<void>
  sendDM: (peerPub: string, plaintext: string) => Promise<void>
  addIncomingDM: (msg: api.DMMessage) => Promise<void>
}

/**
 * Decrypt a single server-side DM message, using the local cache when available.
 * Returns the plaintext, or a placeholder string on failure.
 */
async function resolveMessage(
  msg: api.DMMessage,
  cached: Map<string, CachedMessage>,
  currentUserPub: string,
): Promise<ResolvedDM> {
  const base: Omit<ResolvedDM, 'plaintext'> = {
    id: msg.id,
    senderPub: msg.sender_pub,
    recipientPub: msg.recipient_pub,
    timestamp: msg.timestamp,
  }

  // 1. Check local cache first
  const hit = cached.get(msg.id)
  if (hit) return { ...base, plaintext: hit.plaintext }

  // 2. Our own message — we can't decrypt outbound ciphertext.
  const isMe = msg.sender_pub === currentUserPub
  if (isMe) return { ...base, plaintext: '(sent message — plaintext not cached)' }

  // 3. Received message — try to decrypt
  try {
    const messageType = msg.message_type ?? 2
    const plaintext = await signalManager.decrypt(msg.sender_pub, msg.ciphertext_b64, messageType)
    // Cache it so we don't decrypt again
    const peerKey = msg.sender_pub
    await cachePut({ id: msg.id, plaintext, senderPub: msg.sender_pub, recipientPub: msg.recipient_pub, peerKey, timestamp: msg.timestamp })
    return { ...base, plaintext }
  } catch (err) {
    console.error('[DM decrypt failed]', {
      messageId: msg.id,
      senderPub: msg.sender_pub.slice(0, 20) + '…',
      error: err instanceof Error ? err.message : String(err),
    })
    return { ...base, plaintext: '<decryption failed>' }
  }
}

export const useDMStore = create<DMState>((set, get) => ({
  conversations: [],
  selectedPeer: null,
  messages: {},
  _myPub: '',

  setMyPub: (pub: string) => set({ _myPub: pub }),

  loadConversations: async () => {
    try {
      const peers = await api.listDMConversations()
      set({
        conversations: (peers || []).map(p => ({ peerPub: p })),
      })
    } catch { /* not authenticated yet */ }
  },

  selectPeer: (pub: string) => {
    set({ selectedPeer: pub })
    get().loadDMHistory(pub)
  },

  loadDMHistory: async (peerPub: string) => {
    try {
      const serverMsgs = (await api.getDMs(peerPub)) || []
      const currentUser = get()._myPub
      const ids = serverMsgs.map(m => String(m.id))
      const cached = await cacheGetMany(ids)
      const resolved = await Promise.all(serverMsgs.map(m => resolveMessage(m, cached, currentUser)))
      set({ messages: { ...get().messages, [peerPub]: resolved } })
    } catch { /* ignore */ }
  },

  sendDM: async (peerPub: string, plaintext: string) => {
    const currentUser = get()._myPub
    await signalManager.init(currentUser)
    const { messageType, body } = await signalManager.encrypt(peerPub, plaintext)
    const msg = await api.sendDM(peerPub, body, messageType)

    // Cache our own plaintext
    await cachePut({
      id: msg.id,
      plaintext,
      senderPub: msg.sender_pub,
      recipientPub: msg.recipient_pub,
      peerKey: peerPub,
      timestamp: msg.timestamp,
    })

    const resolved: ResolvedDM = {
      id: msg.id,
      senderPub: msg.sender_pub,
      recipientPub: msg.recipient_pub,
      timestamp: msg.timestamp,
      plaintext,
    }
    const existing = get().messages[peerPub] || []
    set({ messages: { ...get().messages, [peerPub]: [...existing, resolved] } })

    if (!get().conversations.find(c => c.peerPub === peerPub)) {
      set({ conversations: [...get().conversations, { peerPub }] })
    }
  },

  addIncomingDM: async (msg: api.DMMessage) => {
    const peer = msg.sender_pub
    const existing = get().messages[peer] || []
    if (existing.find(m => m.id === msg.id)) return

    const currentUser = get()._myPub
    const cached = await cacheGetMany([msg.id])
    const resolved = await resolveMessage(msg, cached, currentUser)
    const updated = [...(get().messages[peer] || []), resolved]
    set({ messages: { ...get().messages, [peer]: updated } })

    if (!get().conversations.find(c => c.peerPub === peer)) {
      set({ conversations: [...get().conversations, { peerPub: peer }] })
    }
  },
}))

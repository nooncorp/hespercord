import { create } from 'zustand'
import * as api from '../lib/api'
import {
  generateKeypair,
  edPrivateToX25519,
  edPublicToX25519,
  encryptPrivateKey,
  decryptPrivateKey,
  toBase64,
  fromBase64,
} from '../lib/crypto'
import { signalManager } from '../lib/signal'
import { useDMStore } from './dms'

interface Identity {
  edPriv: Uint8Array
  edPub: Uint8Array
  xPriv: Uint8Array
  xPub: Uint8Array
}

interface AuthState {
  loading: boolean
  authenticated: boolean
  isNew: boolean
  discordId: string
  username: string
  avatarUrl: string | null
  identity: Identity | null
  encryptedBlob: Uint8Array | null

  checkAuth: () => Promise<void>
  signup: (password: string) => Promise<void>
  unlock: (password: string) => Promise<void>
  logout: () => Promise<void>
  downloadKeypair: () => void
}

export const useAuthStore = create<AuthState>((set, get) => ({
  loading: true,
  authenticated: false,
  isNew: false,
  discordId: '',
  username: '',
  avatarUrl: null,
  identity: null,
  encryptedBlob: null,

  checkAuth: async () => {
    try {
      const me = await api.getMe()
      if (me.is_new) {
        set({
          loading: false,
          authenticated: true,
          isNew: true,
          discordId: me.discord_id,
          username: me.username,
          avatarUrl: me.avatar_url || null,
        })
      } else {
        set({
          loading: false,
          authenticated: true,
          isNew: false,
          discordId: me.discord_id,
          username: me.username,
          avatarUrl: me.avatar_url || null,
          encryptedBlob: me.encrypted_privkey ? fromBase64(me.encrypted_privkey) : null,
        })
      }
    } catch {
      set({ loading: false, authenticated: false })
    }
  },

  signup: async (password: string) => {
    const { privKey, pubKey } = generateKeypair()
    const xPriv = edPrivateToX25519(privKey)
    const xPub = edPublicToX25519(pubKey)

    const { encrypted, salt, iterations } = await encryptPrivateKey(privKey, password)

    await api.signup({
      ed25519_pub: toBase64(pubKey),
      x25519_pub: toBase64(xPub),
      encrypted_privkey: toBase64(encrypted),
      key_salt: toBase64(salt),
      key_iterations: iterations,
    })

    const pubB64 = toBase64(pubKey)
    await signalManager.init(pubB64, { xPriv, xPub })
    useDMStore.getState().setMyPub(pubB64)

    const me = await api.getMe()
    set({
      isNew: false,
      identity: { edPriv: privKey, edPub: pubKey, xPriv, xPub },
      avatarUrl: me.avatar_url || null,
    })
  },

  unlock: async (password: string) => {
    const me = await api.getMe()
    if (me.is_new || !me.encrypted_privkey || !me.key_salt) {
      throw new Error('No encrypted key found')
    }

    const encrypted = fromBase64(me.encrypted_privkey)
    const salt = fromBase64(me.key_salt!)
    const iterations = me.key_iterations!

    const privKey = await decryptPrivateKey(encrypted, password, salt, iterations)
    const pubKey = fromBase64(me.ed25519_pub!)
    const xPriv = edPrivateToX25519(privKey)
    const xPub = edPublicToX25519(pubKey)

    await signalManager.init(me.ed25519_pub!, { xPriv, xPub })
    useDMStore.getState().setMyPub(me.ed25519_pub!)

    set({
      identity: { edPriv: privKey, edPub: pubKey, xPriv, xPub },
    })
  },

  logout: async () => {
    await api.logout()
    set({
      authenticated: false,
      isNew: false,
      discordId: '',
      username: '',
      avatarUrl: null,
      identity: null,
      encryptedBlob: null,
    })
  },

  downloadKeypair: () => {
    const { identity } = get()
    if (!identity) return
    const data = {
      ed25519_private: toBase64(identity.edPriv),
      ed25519_public: toBase64(identity.edPub),
      x25519_private: toBase64(identity.xPriv),
      x25519_public: toBase64(identity.xPub),
    }
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'hespercord-keypair.json'
    a.click()
    URL.revokeObjectURL(url)
  },
}))

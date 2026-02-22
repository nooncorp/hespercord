/**
 * Signal Protocol (X3DH + Double Ratchet) for 1:1 DMs via @getmaapp/signal-wasm.
 *
 * The Signal identity key is derived from the app's X25519 key pair (itself
 * derived from the Ed25519 app identity). This means recovering the app
 * keypair from the server-side encrypted backup automatically recovers the
 * Signal identity — no separate backup needed.
 *
 * Pre-keys, sessions, and ratchet state are ephemeral and stored in IndexedDB.
 * Losing them just means peers need to re-establish sessions.
 */

import initWasm, { SignalClient, type WasmCiphertext } from '@getmaapp/signal-wasm'
import { toBase64, fromBase64 } from './crypto'
import { sha256 } from '@noble/hashes/sha2.js'
import * as api from './api'

const DEVICE_ID = 1
const DB_NAME = 'hespercord-signal'
const DB_VERSION = 1
const PREKEY_COUNT = 100

const STORE_META = 'meta'
const STORE_SESSIONS = 'sessions'
const STORE_PREKEYS = 'prekeys'
const STORE_SIGNED_PREKEYS = 'signed_prekeys'
const STORE_KYBER_PREKEYS = 'kyber_prekeys'

let wasmInitPromise: Promise<void> | null = null

async function ensureWasm(): Promise<void> {
  if (wasmInitPromise == null) {
    wasmInitPromise = (async () => {
      await initWasm()
    })()
  }
  await wasmInitPromise
}

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve(req.result)
    req.onupgradeneeded = (e) => {
      const db = (e.target as IDBOpenDBRequest).result
      if (!db.objectStoreNames.contains(STORE_META)) db.createObjectStore(STORE_META)
      if (!db.objectStoreNames.contains(STORE_SESSIONS)) db.createObjectStore(STORE_SESSIONS)
      if (!db.objectStoreNames.contains(STORE_PREKEYS)) db.createObjectStore(STORE_PREKEYS)
      if (!db.objectStoreNames.contains(STORE_SIGNED_PREKEYS)) db.createObjectStore(STORE_SIGNED_PREKEYS)
      if (!db.objectStoreNames.contains(STORE_KYBER_PREKEYS)) db.createObjectStore(STORE_KYBER_PREKEYS)
    }
  })
}

function idbGet<T>(storeName: string, key: string | number): Promise<T | undefined> {
  return openDB().then((db) => {
    return new Promise((resolve, reject) => {
      const tx = db.transaction(storeName, 'readonly')
      const store = tx.objectStore(storeName)
      const req = store.get(key)
      req.onerror = () => { db.close(); reject(req.error) }
      req.onsuccess = () => { db.close(); resolve(req.result as T | undefined) }
    })
  })
}

function idbPut(storeName: string, key: string | number, value: unknown): Promise<void> {
  return openDB().then((db) => {
    return new Promise((resolve, reject) => {
      const tx = db.transaction(storeName, 'readwrite')
      const store = tx.objectStore(storeName)
      const req = store.put(value, key)
      req.onerror = () => { db.close(); reject(req.error) }
      req.onsuccess = () => { db.close(); resolve() }
    })
  })
}

function idbGetAllKeys(storeName: string): Promise<(string | number)[]> {
  return openDB().then((db) => {
    return new Promise((resolve, reject) => {
      const tx = db.transaction(storeName, 'readonly')
      const store = tx.objectStore(storeName)
      const req = store.getAllKeys()
      req.onerror = () => { db.close(); reject(req.error) }
      req.onsuccess = () => { db.close(); resolve(req.result as (string | number)[]) }
    })
  })
}

function bytesToStore(b: Uint8Array): ArrayBuffer {
  const ab = new ArrayBuffer(b.length)
  new Uint8Array(ab).set(b)
  return ab
}

function storeToBytes(ab: ArrayBuffer): Uint8Array {
  return new Uint8Array(ab)
}

/** libsignal serializes Curve25519 public keys with a 0x05 type prefix. */
function toSignalPub(rawX25519Pub: Uint8Array): Uint8Array {
  const buf = new Uint8Array(33)
  buf[0] = 0x05
  buf.set(rawX25519Pub, 1)
  return buf
}

/** Deterministic 14-bit registration ID from private key material. */
function deriveRegistrationId(privKey: Uint8Array): number {
  const tag = new TextEncoder().encode('hespercord-signal-regid')
  const input = new Uint8Array(tag.length + privKey.length)
  input.set(tag)
  input.set(privKey, tag.length)
  const h = sha256(input)
  return ((h[0] << 8 | h[1]) % 16380) + 1
}

interface IdentityMeta {
  identityPubB64: string
  identityPrivB64: string
  registrationId: number
  nextPrekeyId: number
  nextSignedPrekeyId: number
  nextKyberPrekeyId: number
}

export interface AppKeys {
  xPriv: Uint8Array
  xPub: Uint8Array
}

class SignalManager {
  private client: SignalClient | null = null
  private userId: string | null = null
  private initialized = false

  /**
   * Initialize or restore the Signal client.
   *
   * @param userId  The user's Ed25519 public key (base64) — used as the Signal address.
   * @param appKeys The app's X25519 key pair. Required on first init; on subsequent
   *                calls in the same page load the cached client is returned immediately.
   */
  async init(userId: string, appKeys?: AppKeys): Promise<void> {
    if (this.initialized && this.userId === userId) return
    await ensureWasm()

    this.userId = userId

    // Derive the expected Signal identity from the app's X25519 keys
    const derivedPub = appKeys ? toSignalPub(appKeys.xPub) : null
    const derivedPubB64 = derivedPub ? toBase64(derivedPub) : null

    const loaded = await this.loadState()
    if (loaded) {
      // If we have app keys, verify the stored Signal identity matches
      if (derivedPubB64) {
        const meta = await idbGet<IdentityMeta>(STORE_META, 'identity')
        if (meta && meta.identityPubB64 !== derivedPubB64) {
          console.warn('[Signal] Stored identity does not match app identity — re-initializing')
          await this.clearAllStores()
          // fall through to fresh init below
        } else {
          this.initialized = true
          return
        }
      } else {
        this.initialized = true
        return
      }
    }

    // Fresh init — we need the app keys to derive the Signal identity
    if (!appKeys || !derivedPub) {
      throw new Error('SignalManager: cannot create fresh identity without app keys')
    }

    const signalPriv = appKeys.xPriv
    const regId = deriveRegistrationId(signalPriv)

    this.client = SignalClient.restore(
      derivedPub, signalPriv, regId, userId, DEVICE_ID,
      1, 1, 1, // nextPrekeyId, nextSignedPrekeyId, nextKyberPrekeyId
    )

    const prekeys = this.client.generate_pre_keys(PREKEY_COUNT)
    const signedPreKey = this.client.generate_signed_pre_key()
    const kyberPreKey = this.client.generate_kyber_pre_key()

    for (const pk of prekeys) {
      const record = await this.client.export_pre_key(pk.id)
      if (record) await idbPut(STORE_PREKEYS, pk.id, bytesToStore(record))
    }
    const spkRecord = await this.client.export_signed_pre_key(signedPreKey.id)
    if (spkRecord) await idbPut(STORE_SIGNED_PREKEYS, signedPreKey.id, bytesToStore(spkRecord))
    const kpkRecord = await this.client.export_kyber_pre_key(kyberPreKey.id)
    if (kpkRecord) await idbPut(STORE_KYBER_PREKEYS, kyberPreKey.id, bytesToStore(kpkRecord))

    const meta: IdentityMeta = {
      identityPubB64: derivedPubB64!,
      identityPrivB64: toBase64(signalPriv),
      registrationId: regId,
      nextPrekeyId: this.client.get_next_pre_key_id(),
      nextSignedPrekeyId: this.client.get_next_signed_pre_key_id(),
      nextKyberPrekeyId: this.client.get_next_kyber_pre_key_id(),
    }
    await idbPut(STORE_META, 'userId', userId)
    await idbPut(STORE_META, 'identity', meta)

    const oneTimePrekeys = prekeys.map((pk) => ({ prekey_id: pk.id, prekey_pub: toBase64(pk.public_key) }))
    await api.uploadPreKeys({
      pub_key: userId,
      registration_id: regId,
      identity_key_pub: derivedPubB64!,
      signed_prekey_id: signedPreKey.id,
      signed_prekey_pub: toBase64(signedPreKey.public_key),
      signed_prekey_sig: toBase64(signedPreKey.signature),
      one_time_prekeys: oneTimePrekeys,
      kyber_prekey_id: kyberPreKey.id,
      kyber_prekey_pub: toBase64(kyberPreKey.public_key),
      kyber_prekey_sig: toBase64(kyberPreKey.signature),
    })

    this.initialized = true
  }

  private async loadState(): Promise<boolean> {
    const userIdStored = await idbGet<string>(STORE_META, 'userId')
    const meta = await idbGet<IdentityMeta>(STORE_META, 'identity')
    if (!userIdStored || !meta || userIdStored !== this.userId) return false

    this.client = SignalClient.restore(
      fromBase64(meta.identityPubB64),
      fromBase64(meta.identityPrivB64),
      meta.registrationId,
      this.userId!,
      DEVICE_ID,
      meta.nextPrekeyId,
      meta.nextSignedPrekeyId,
      meta.nextKyberPrekeyId,
    )

    const prekeyIds = await idbGetAllKeys(STORE_PREKEYS)
    for (const id of prekeyIds) {
      const rec = await idbGet<ArrayBuffer>(STORE_PREKEYS, id)
      if (rec) await this.client!.import_pre_key(id as number, storeToBytes(rec))
    }
    const spkIds = await idbGetAllKeys(STORE_SIGNED_PREKEYS)
    for (const id of spkIds) {
      const rec = await idbGet<ArrayBuffer>(STORE_SIGNED_PREKEYS, id)
      if (rec) await this.client!.import_signed_pre_key(id as number, storeToBytes(rec))
    }
    const kpkIds = await idbGetAllKeys(STORE_KYBER_PREKEYS)
    for (const id of kpkIds) {
      const rec = await idbGet<ArrayBuffer>(STORE_KYBER_PREKEYS, id)
      if (rec) await this.client!.import_kyber_pre_key(id as number, storeToBytes(rec))
    }

    const sessionKeys = await idbGetAllKeys(STORE_SESSIONS)
    for (const peerKey of sessionKeys) {
      if (typeof peerKey !== 'string') continue
      const sessionBytes = await idbGet<ArrayBuffer>(STORE_SESSIONS, peerKey)
      if (sessionBytes) await this.client!.import_session(peerKey, DEVICE_ID, storeToBytes(sessionBytes))
    }

    return true
  }

  private async clearAllStores(): Promise<void> {
    const db = await openDB()
    const storeNames = [STORE_META, STORE_SESSIONS, STORE_PREKEYS, STORE_SIGNED_PREKEYS, STORE_KYBER_PREKEYS]
    const tx = db.transaction(storeNames, 'readwrite')
    for (const name of storeNames) tx.objectStore(name).clear()
    await new Promise<void>((resolve, reject) => {
      tx.oncomplete = () => { db.close(); resolve() }
      tx.onerror = () => { db.close(); reject(tx.error) }
    })
    this.client = null
    this.initialized = false
  }

  async ensureSession(peerUserId: string): Promise<void> {
    if (!this.client || !this.userId) throw new Error('SignalManager not initialized')
    const hasSession = await this.client.has_session(peerUserId, DEVICE_ID)
    if (hasSession) return

    let bundle: api.PreKeyBundleResponse
    try {
      bundle = await api.getPreKeys(peerUserId)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      if (msg.includes('404') || msg.includes('no pre-key bundle') || msg.includes('not found')) {
        throw new Error('Recipient has not set up E2EE yet. Ask them to log in to the app.')
      }
      throw e
    }
    if (!bundle) throw new Error('Recipient has not set up E2EE yet. Ask them to log in to the app.')

    const hasKyber =
      bundle.kyber_prekey_pub != null &&
      bundle.kyber_prekey_pub.length > 0 &&
      bundle.kyber_prekey_sig != null &&
      bundle.kyber_prekey_sig.length > 0
    if (!hasKyber) {
      throw new Error(
        'Recipient needs to log out and log back in to enable E2EE. Their client is on an older version.',
      )
    }

    const identityKey = fromBase64(bundle.identity_key_pub)
    const signedPrekey = fromBase64(bundle.signed_prekey_pub)
    const signedPrekeySig = fromBase64(bundle.signed_prekey_sig)
    const kyberPrekeyId = bundle.kyber_prekey_id ?? 0
    const kyberPrekey = fromBase64(bundle.kyber_prekey_pub!)
    const kyberPrekeySig = fromBase64(bundle.kyber_prekey_sig!)

    let prekeyId: number | null = null
    let prekey: Uint8Array | null = null
    if (bundle.one_time_prekey) {
      prekeyId = bundle.one_time_prekey.prekey_id
      prekey = fromBase64(bundle.one_time_prekey.prekey_pub)
    }

    try {
      await this.client.process_pre_key_bundle(
        peerUserId,
        DEVICE_ID,
        bundle.registration_id,
        identityKey,
        bundle.signed_prekey_id,
        signedPrekey,
        signedPrekeySig,
        prekeyId ?? undefined,
        prekey ?? undefined,
        kyberPrekeyId,
        kyberPrekey,
        kyberPrekeySig,
      )
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      throw new Error(`Could not establish secure session: ${msg}. The recipient may need to log in again.`)
    }
    await this.saveSession(peerUserId)
  }

  private async saveSession(peerUserId: string): Promise<void> {
    if (!this.client) return
    const bytes = await this.client.export_session(peerUserId, DEVICE_ID)
    if (bytes) await idbPut(STORE_SESSIONS, peerUserId, bytesToStore(bytes))
  }

  async encrypt(peerUserId: string, plaintext: string): Promise<{ messageType: number; body: string }> {
    if (!this.client) throw new Error('SignalManager not initialized')
    await this.ensureSession(peerUserId)
    const plaintextBytes = new TextEncoder().encode(plaintext)
    try {
      const ct: WasmCiphertext = await this.client.encrypt_message(peerUserId, DEVICE_ID, plaintextBytes)
      await this.saveSession(peerUserId)
      return { messageType: ct.message_type, body: toBase64(ct.body) }
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      throw new Error(`Encryption failed: ${msg}. Try starting a new conversation or ask the recipient to log in again.`)
    }
  }

  async decrypt(senderUserId: string, ciphertextB64: string, messageType: number): Promise<string> {
    if (!this.client) throw new Error('SignalManager not initialized')
    const ciphertext = fromBase64(ciphertextB64)
    const plaintext = await this.client.decrypt_message(senderUserId, DEVICE_ID, ciphertext, messageType)
    await this.saveSession(senderUserId)
    return new TextDecoder().decode(plaintext)
  }

  async saveState(): Promise<void> {
    if (!this.client) return
    const sessionKeys = await idbGetAllKeys(STORE_SESSIONS)
    for (const peerKey of sessionKeys) {
      if (typeof peerKey !== 'string') continue
      const bytes = await this.client.export_session(peerKey, DEVICE_ID)
      if (bytes) await idbPut(STORE_SESSIONS, peerKey, bytesToStore(bytes))
    }
  }
}

export const signalManager = new SignalManager()

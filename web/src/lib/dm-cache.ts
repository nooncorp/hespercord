/**
 * IndexedDB cache for decrypted DM plaintext.
 *
 * The server only stores Signal ciphertext. Because Signal encrypts for the
 * recipient only, the sender cannot decrypt their own outbound messages.
 * We cache plaintext locally so messages survive page reloads.
 *
 * Schema:  message-id (string PK) -> { id, plaintext, senderPub, recipientPub, timestamp }
 */

const DB_NAME = 'hespercord-dm-cache'
const DB_VERSION = 1
const STORE_NAME = 'messages'

function openDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(DB_NAME, DB_VERSION)
    req.onerror = () => reject(req.error)
    req.onsuccess = () => resolve(req.result)
    req.onupgradeneeded = (e) => {
      const db = (e.target as IDBOpenDBRequest).result
      if (!db.objectStoreNames.contains(STORE_NAME)) {
        const store = db.createObjectStore(STORE_NAME, { keyPath: 'id' })
        store.createIndex('byPeer', 'peerKey', { unique: false })
      }
    }
  })
}

export interface CachedMessage {
  id: string
  plaintext: string
  senderPub: string
  recipientPub: string
  peerKey: string   // the "other" party's pub — used as an index for per-conversation queries
  timestamp: string
}

export async function cachePut(msg: CachedMessage): Promise<void> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    tx.objectStore(STORE_NAME).put(msg)
    tx.oncomplete = () => { db.close(); resolve() }
    tx.onerror = () => { db.close(); reject(tx.error) }
  })
}

export async function cachePutMany(msgs: CachedMessage[]): Promise<void> {
  if (msgs.length === 0) return
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readwrite')
    const store = tx.objectStore(STORE_NAME)
    for (const m of msgs) store.put(m)
    tx.oncomplete = () => { db.close(); resolve() }
    tx.onerror = () => { db.close(); reject(tx.error) }
  })
}

export async function cacheGet(id: string): Promise<CachedMessage | undefined> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const req = tx.objectStore(STORE_NAME).get(id)
    req.onsuccess = () => { db.close(); resolve(req.result as CachedMessage | undefined) }
    req.onerror = () => { db.close(); reject(req.error) }
  })
}

export async function cacheGetMany(ids: string[]): Promise<Map<string, CachedMessage>> {
  if (ids.length === 0) return new Map()
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE_NAME, 'readonly')
    const store = tx.objectStore(STORE_NAME)
    const map = new Map<string, CachedMessage>()
    let pending = ids.length
    for (const id of ids) {
      const req = store.get(id)
      req.onsuccess = () => {
        if (req.result) map.set(id, req.result as CachedMessage)
        if (--pending === 0) { db.close(); resolve(map) }
      }
      req.onerror = () => {
        if (--pending === 0) { db.close(); resolve(map) }
      }
    }
    tx.onerror = () => { db.close(); reject(tx.error) }
  })
}

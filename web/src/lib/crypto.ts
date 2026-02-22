import { xchacha20poly1305 } from '@noble/ciphers/chacha.js'
import { randomBytes } from '@noble/ciphers/utils.js'
import { x25519, ed25519 } from '@noble/curves/ed25519.js'
import { sha256 } from '@noble/hashes/sha2.js'

export const GUILD_KEY_SIZE = 32
const NONCE_SIZE = 24

export function generateGuildKey(): Uint8Array {
  return randomBytes(GUILD_KEY_SIZE)
}

export function encryptMessage(guildKey: Uint8Array, plaintext: Uint8Array): Uint8Array {
  const nonce = randomBytes(NONCE_SIZE)
  const cipher = xchacha20poly1305(guildKey, nonce)
  const ct = cipher.encrypt(plaintext)
  const result = new Uint8Array(nonce.length + ct.length)
  result.set(nonce)
  result.set(ct, nonce.length)
  return result
}

export function decryptMessage(guildKey: Uint8Array, sealed: Uint8Array): Uint8Array {
  if (sealed.length < NONCE_SIZE) throw new Error('ciphertext too short')
  const nonce = sealed.slice(0, NONCE_SIZE)
  const ct = sealed.slice(NONCE_SIZE)
  const cipher = xchacha20poly1305(guildKey, nonce)
  return cipher.decrypt(ct)
}

export function sealKey(
  key: Uint8Array,
  senderXPriv: Uint8Array,
  recipientXPub: Uint8Array
): Uint8Array {
  const shared = x25519.getSharedSecret(senderXPriv, recipientXPub)
  const symKey = sha256(shared)
  return encryptMessage(symKey, key)
}

export function unsealKey(
  sealed: Uint8Array,
  recipientXPriv: Uint8Array,
  senderXPub: Uint8Array
): Uint8Array {
  const shared = x25519.getSharedSecret(recipientXPriv, senderXPub)
  const symKey = sha256(shared)
  return decryptMessage(symKey, sealed)
}

export function signMessagePayload(
  privateKey: Uint8Array,
  channelId: string,
  content: string,
  seq: number
): Uint8Array {
  const payload = new TextEncoder().encode(`${channelId}\n${content}\n${seq}`)
  return ed25519.sign(payload, privateKey)
}

export function verifySignature(
  publicKey: Uint8Array,
  channelId: string,
  content: string,
  seq: number,
  signature: Uint8Array
): boolean {
  const payload = new TextEncoder().encode(`${channelId}\n${content}\n${seq}`)
  return ed25519.verify(signature, payload, publicKey)
}

export function generateKeypair() {
  const privKey = ed25519.utils.randomSecretKey()
  const pubKey = ed25519.getPublicKey(privKey)
  return { privKey, pubKey }
}

// Ed25519 private seed -> X25519 private key
export function edPrivateToX25519(edPrivSeed: Uint8Array): Uint8Array {
  return ed25519.utils.toMontgomerySecret(edPrivSeed)
}

// Ed25519 public key -> X25519 public key (birational map)
export function edPublicToX25519(edPub: Uint8Array): Uint8Array {
  return ed25519.utils.toMontgomery(edPub)
}

export async function deriveKeyFromPassword(
  password: string,
  salt: Uint8Array,
  iterations: number
): Promise<CryptoKey> {
  const enc = new TextEncoder()
  const keyMaterial = await crypto.subtle.importKey(
    'raw', enc.encode(password), 'PBKDF2', false, ['deriveKey']
  )
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: salt as BufferSource, iterations, hash: 'SHA-256' },
    keyMaterial,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt']
  )
}

export async function encryptPrivateKey(
  privKey: Uint8Array,
  password: string
): Promise<{ encrypted: Uint8Array; salt: Uint8Array; iterations: number }> {
  const salt = randomBytes(16)
  const iterations = 600000
  const key = await deriveKeyFromPassword(password, salt, iterations)
  const iv = randomBytes(12)
  const ct = await crypto.subtle.encrypt({ name: 'AES-GCM', iv: iv as BufferSource }, key, privKey as BufferSource)
  const encrypted = new Uint8Array(iv.length + ct.byteLength)
  encrypted.set(iv)
  encrypted.set(new Uint8Array(ct), iv.length)
  return { encrypted, salt, iterations }
}

export async function decryptPrivateKey(
  encrypted: Uint8Array,
  password: string,
  salt: Uint8Array,
  iterations: number
): Promise<Uint8Array> {
  const key = await deriveKeyFromPassword(password, salt, iterations)
  const iv = encrypted.slice(0, 12)
  const ct = encrypted.slice(12)
  const pt = await crypto.subtle.decrypt({ name: 'AES-GCM', iv }, key, ct)
  return new Uint8Array(pt)
}

export function toBase64(bytes: Uint8Array): string {
  let binary = ''
  for (const b of bytes) binary += String.fromCharCode(b)
  return btoa(binary)
}

export function fromBase64(str: string): Uint8Array {
  const binary = atob(str)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

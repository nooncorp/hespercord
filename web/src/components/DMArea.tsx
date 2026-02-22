import { useEffect, useRef, useState } from 'react'
import { useDMStore, type ResolvedDM } from '../stores/dms'
import { useAuthStore } from '../stores/auth'
import { toBase64 } from '../lib/crypto'
import * as api from '../lib/api'
import Avatar from './Avatar'

function DMBubble({
  msg,
  senderLabel,
  isMe,
  avatarUrl,
}: {
  msg: ResolvedDM
  senderLabel: string
  isMe: boolean
  avatarUrl: string | null
}) {
  const time = new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return (
    <div className="flex gap-4 py-0.5 hover:bg-bg-message-hover rounded px-2 -mx-2">
      <div className="mt-0.5 shrink-0">
        <Avatar url={isMe ? avatarUrl : null} letter={senderLabel} size="lg" />
      </div>
      <div className="min-w-0 flex-1 py-0.5">
        <div className="flex items-baseline gap-2">
          <span className={`font-semibold text-sm ${isMe ? 'text-bg-accent' : 'text-white'}`}>{senderLabel}</span>
          <span className="text-[11px] text-text-muted">{time}</span>
        </div>
        <div className="text-text-primary text-[15px] leading-relaxed mt-0.5 break-words">{msg.plaintext}</div>
      </div>
    </div>
  )
}

export default function DMArea() {
  const { selectedPeer, messages, sendDM } = useDMStore()
  const { identity, avatarUrl } = useAuthStore()
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [sendError, setSendError] = useState<string | null>(null)
  const [users, setUsers] = useState<Record<string, string>>({})
  const bottomRef = useRef<HTMLDivElement>(null)

  const peerMessages = selectedPeer ? (messages[selectedPeer] || []) : []

  useEffect(() => {
    api.listUsers().then(list => {
      const map: Record<string, string> = {}
      for (const u of (list || [])) map[u.pub_key] = u.name
      setUsers(map)
    }).catch(() => {})
  }, [])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [peerMessages.length])

  if (!selectedPeer) {
    return (
      <div className="flex-1 flex items-center justify-center text-text-muted">
        <div className="text-center">
          <div className="text-5xl mb-4 opacity-20">💬</div>
          <p className="text-lg font-semibold text-text-secondary mb-1">No conversation selected</p>
          <p className="text-sm">Select a friend from the list or click a member in a guild to start chatting.</p>
        </div>
      </div>
    )
  }

  const myPub = identity ? toBase64(identity.edPub) : ''
  const peerName = users[selectedPeer] || selectedPeer.slice(0, 16) + '...'

  const handleSend = async () => {
    if (!text.trim() || sending || !identity || !selectedPeer) return
    setSendError(null)
    setSending(true)
    try {
      await sendDM(selectedPeer, text.trim())
      setText('')
    } catch (e: unknown) {
      const msg = e instanceof Error ? e.message : String(e)
      console.error('DM send error:', e)
      setSendError(msg)
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="flex-1 flex flex-col min-w-0 min-h-0">
      <div className="h-12 px-4 flex items-center border-b border-border-thin shadow-sm shrink-0 gap-2">
        <Avatar url={null} letter={peerName} size="sm" />
        <span className="text-white font-semibold text-sm">{peerName}</span>
      </div>

      <div className="flex-1 overflow-y-auto min-h-0 px-4 py-4 space-y-1">
        {peerMessages.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16">
            <div className="w-16 h-16 rounded-full bg-bg-accent flex items-center justify-center text-2xl font-bold text-white mb-4">
              {peerName.charAt(0).toUpperCase()}
            </div>
            <h3 className="text-white font-bold text-lg mb-1">{peerName}</h3>
            <p className="text-text-muted text-sm">This is the beginning of your direct message history.</p>
          </div>
        ) : (
          peerMessages.map(msg => (
            <DMBubble
              key={msg.id}
              msg={msg}
              senderLabel={users[msg.senderPub] || msg.senderPub.slice(0, 12) + '...'}
              isMe={msg.senderPub === myPub}
              avatarUrl={avatarUrl}
            />
          ))
        )}
        <div ref={bottomRef} />
      </div>

      <div className="px-4 pb-6 shrink-0">
        {sendError && (
          <div className="mb-2 px-3 py-2 rounded bg-red-500/10 border border-red-500/30 text-red-200 text-sm">
            {sendError}
          </div>
        )}
        <div className="bg-bg-input rounded-lg flex items-center px-4">
          <input
            className="flex-1 bg-transparent text-text-primary py-3 text-sm placeholder:text-text-muted"
            placeholder={`Message @${peerName}`}
            value={text}
            onChange={e => { setText(e.target.value); setSendError(null) }}
            onKeyDown={e => { if (e.key === 'Enter' && !e.shiftKey) { e.preventDefault(); handleSend() } }}
            disabled={sending}
          />
          <button
            onClick={handleSend}
            disabled={sending || !text.trim()}
            className="ml-2 text-bg-accent hover:text-bg-accent-hover disabled:text-text-muted transition-colors"
          >
            <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
              <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z" />
            </svg>
          </button>
        </div>
      </div>
    </div>
  )
}

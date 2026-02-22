import { useEffect, useRef, useState } from 'react'
import { useGuildStore } from '../stores/guilds'
import { useMessageStore, type DecryptedMessage } from '../stores/messages'
import { useAuthStore } from '../stores/auth'
import { toBase64 } from '../lib/crypto'
import * as api from '../lib/api'
import Avatar from './Avatar'

export default function MessageArea() {
  const { selectedGuildId, selectedChannelId, channels, getAvatarUrl } = useGuildStore()
  const { messages, loadMessages } = useMessageStore()
  const { identity } = useAuthStore()
  const bottomRef = useRef<HTMLDivElement>(null)
  const [users, setUsers] = useState<Record<string, string>>({})

  const channelName = selectedGuildId
    ? channels[selectedGuildId]?.find(c => c.id === selectedChannelId)?.name || 'general'
    : 'DMs'

  const key = `${selectedGuildId}:${selectedChannelId}`
  const channelMessages = messages[key] || []

  useEffect(() => {
    api.listUsers().then(list => {
      const map: Record<string, string> = {}
      for (const u of (list || [])) map[u.pub_key] = u.name
      setUsers(map)
    }).catch(() => {})
  }, [])

  useEffect(() => {
    if (selectedGuildId && selectedChannelId && identity) {
      loadMessages(selectedGuildId, selectedChannelId)
    }
  }, [selectedGuildId, selectedChannelId, identity])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [channelMessages.length])

  if (!selectedGuildId) {
    return (
      <div className="flex-1 flex items-center justify-center text-text-muted">
        Select a guild to get started
      </div>
    )
  }

  const myPub = identity ? toBase64(identity.edPub) : ''

  return (
    <div className="flex-1 flex flex-col min-w-0 min-h-0">
      <div className="h-12 px-4 flex items-center border-b border-border-thin shadow-sm shrink-0">
        <span className="text-text-muted mr-1.5">#</span>
        <span className="text-white font-semibold text-sm">{channelName}</span>
      </div>

      <div className="flex-1 overflow-y-auto min-h-0 px-4 py-4 space-y-1">
        {channelMessages.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-16">
            <div className="text-4xl mb-3 opacity-30">#</div>
            <h3 className="text-white font-bold text-lg mb-1">Welcome to #{channelName}!</h3>
            <p className="text-text-muted text-sm">This is the start of the channel. Send a message to get things going.</p>
          </div>
        ) : (
          channelMessages.map(msg => (
            <MessageRow
              key={msg.id}
              msg={msg}
              isMe={msg.senderPub === myPub}
              users={users}
              avatarUrl={selectedGuildId ? getAvatarUrl(selectedGuildId, msg.senderPub) : null}
            />
          ))
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}

function MessageRow({
  msg,
  isMe,
  users,
  avatarUrl,
}: {
  msg: DecryptedMessage
  isMe: boolean
  users: Record<string, string>
  avatarUrl: string | null
}) {
  const senderLabel = users[msg.senderPub] || msg.senderPub.slice(0, 12) + '...'
  const time = new Date(msg.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })

  return (
    <div className="flex gap-4 py-0.5 hover:bg-bg-message-hover rounded px-2 -mx-2 group">
      <div className="mt-0.5 shrink-0">
        <Avatar url={avatarUrl} letter={senderLabel} size="lg" />
      </div>
      <div className="min-w-0 flex-1 py-0.5">
        <div className="flex items-baseline gap-2">
          <span className={`font-semibold text-sm ${isMe ? 'text-bg-accent' : 'text-white'}`}>
            {senderLabel}
          </span>
          <span className="text-[11px] text-text-muted">{time}</span>
        </div>
        {msg.error ? (
          <div className="text-danger text-sm italic mt-0.5">&lt;{msg.error}&gt;</div>
        ) : (
          <div className="text-text-primary text-[15px] leading-relaxed mt-0.5 break-words">{msg.content}</div>
        )}
      </div>
    </div>
  )
}

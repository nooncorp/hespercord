import { useGuildStore } from '../stores/guilds'
import { useDMStore } from '../stores/dms'
import { useAuthStore } from '../stores/auth'
import { useEffect, useState } from 'react'
import * as api from '../lib/api'
import Avatar from './Avatar'

export default function ChannelList() {
  const { selectedGuildId, channels, selectedChannelId, selectChannel, createChannel, guilds } = useGuildStore()
  const { conversations, selectedPeer, selectPeer } = useDMStore()
  const { username, identity } = useAuthStore()
  const [newChannel, setNewChannel] = useState('')
  const [showAdd, setShowAdd] = useState(false)
  const [users, setUsers] = useState<Record<string, string>>({})

  const guild = guilds.find(g => g.id === selectedGuildId)

  useEffect(() => {
    api.listUsers().then(list => {
      const map: Record<string, string> = {}
      for (const u of (list || [])) map[u.pub_key] = u.name
      setUsers(map)
    }).catch(() => {})
  }, [])

  if (!selectedGuildId) {
    return (
      <div className="w-60 bg-bg-secondary flex flex-col shrink-0">
        <div className="h-12 px-4 flex items-center border-b border-border-thin font-semibold text-white shadow-sm text-sm">
          Direct Messages
        </div>
        <div className="flex-1 overflow-y-auto px-2 py-2">
          {conversations.length === 0 ? (
            <div className="text-text-muted text-xs px-2 py-4">No conversations yet</div>
          ) : (
            conversations.map(c => {
              const name = users[c.peerPub] || c.peerPub.slice(0, 12) + '...'
              return (
                <button
                  key={c.peerPub}
                  onClick={() => selectPeer(c.peerPub)}
                  className={`w-full text-left px-2 py-1.5 rounded text-sm flex items-center gap-3 mb-0.5
                    ${selectedPeer === c.peerPub ? 'bg-bg-message-hover text-white' : 'text-channel-default hover:text-channel-hover hover:bg-bg-message-hover'}`}
                >
                  <span className="w-8 h-8 rounded-full bg-bg-accent flex items-center justify-center text-xs font-semibold text-white shrink-0">
                    {name.charAt(0).toUpperCase()}
                  </span>
                  <span className="truncate text-sm">{name}</span>
                </button>
              )
            })
          )}
        </div>
        <UserPanel username={username} identity={identity} />
      </div>
    )
  }

  const guildChannels = channels[selectedGuildId] || []

  const handleAddChannel = () => {
    if (newChannel.trim()) {
      createChannel(selectedGuildId, newChannel.trim())
      setNewChannel('')
      setShowAdd(false)
    }
  }

  return (
    <div className="w-60 bg-bg-secondary flex flex-col shrink-0">
      <div className="h-12 px-4 flex items-center border-b border-border-thin font-semibold text-white shadow-sm text-sm">
        <span className="truncate">{guild?.name || 'Guild'}</span>
      </div>

      <div className="flex-1 overflow-y-auto px-2 py-3">
        <div className="flex items-center justify-between px-2 mb-1">
          <span className="text-[11px] font-semibold text-text-muted uppercase tracking-wide">
            Text Channels
          </span>
          <button
            onClick={() => setShowAdd(!showAdd)}
            className="text-text-muted hover:text-text-primary text-lg leading-none"
            title="Create Channel"
          >
            +
          </button>
        </div>

        {showAdd && (
          <div className="px-2 mb-2">
            <input
              className="w-full bg-bg-input text-text-primary rounded px-3 py-1.5 text-xs"
              placeholder="channel-name"
              value={newChannel}
              onChange={e => setNewChannel(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') handleAddChannel(); if (e.key === 'Escape') setShowAdd(false) }}
              autoFocus
            />
          </div>
        )}

        {guildChannels.map(ch => (
          <button
            key={ch.id}
            onClick={() => selectChannel(ch.id)}
            className={`w-full text-left px-2 py-1.5 rounded text-sm flex items-center gap-1.5 mb-0.5
              ${selectedChannelId === ch.id ? 'bg-bg-message-hover text-white font-medium' : 'text-channel-default hover:text-channel-hover hover:bg-bg-message-hover'}`}
          >
            <span className="text-text-muted text-base">#</span>
            <span className="truncate">{ch.name}</span>
          </button>
        ))}
      </div>

      <UserPanel username={username} identity={identity} />
    </div>
  )
}

function UserPanel({ username, identity }: { username: string; identity: any }) {
  const { avatarUrl, downloadKeypair, logout } = useAuthStore()

  return (
    <div className="bg-bg-tertiary px-3 py-2.5 flex items-center gap-2 border-t border-border-thin">
      <Avatar url={avatarUrl} letter={username} size="md" />
      <div className="flex-1 min-w-0">
        <div className="text-sm font-semibold text-white truncate">{username}</div>
        <div className="text-[10px] text-text-muted truncate">
          {identity ? 'Unlocked' : 'Locked'}
        </div>
      </div>
      <button onClick={downloadKeypair} className="text-text-muted hover:text-text-primary p-1" title="Download Keys">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4M7 10l5 5 5-5M12 15V3"/>
        </svg>
      </button>
      <button onClick={logout} className="text-text-muted hover:text-danger p-1" title="Logout">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
          <path d="M9 21H5a2 2 0 01-2-2V5a2 2 0 012-2h4M16 17l5-5-5-5M21 12H9"/>
        </svg>
      </button>
    </div>
  )
}

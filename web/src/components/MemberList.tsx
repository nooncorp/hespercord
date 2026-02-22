import { useEffect, useState } from 'react'
import { useGuildStore } from '../stores/guilds'
import { useAuthStore } from '../stores/auth'
import { useDMStore } from '../stores/dms'
import { toBase64 } from '../lib/crypto'
import * as api from '../lib/api'
import Avatar from './Avatar'

export default function MemberList() {
  const { selectedGuildId, guilds, inviteMember, kickMember, getAvatarUrl } = useGuildStore()
  const { identity } = useAuthStore()
  const [members, setMembers] = useState<string[]>([])
  const [users, setUsers] = useState<Record<string, string>>({})
  const [showInvite, setShowInvite] = useState(false)
  const [invitePub, setInvitePub] = useState('')

  const guild = guilds.find(g => g.id === selectedGuildId)
  const myPub = identity ? toBase64(identity.edPub) : ''
  const isOwner = guild?.owner_pub === myPub

  useEffect(() => {
    api.listUsers().then(list => {
      const map: Record<string, string> = {}
      for (const u of (list || [])) map[u.pub_key] = u.name
      setUsers(map)
    }).catch(() => {})
  }, [])

  useEffect(() => {
    if (selectedGuildId && identity) {
      api.listMembers(selectedGuildId).then(setMembers).catch(() => {})
    }
  }, [selectedGuildId, identity, guilds])

  if (!selectedGuildId) return null

  const handleInvite = async () => {
    if (!invitePub.trim() || !selectedGuildId) return
    try {
      await inviteMember(selectedGuildId, invitePub.trim())
      setInvitePub('')
      setShowInvite(false)
      api.listMembers(selectedGuildId).then(setMembers)
    } catch (e: any) {
      alert('Invite failed: ' + e.message)
    }
  }

  const handleKick = async (pub: string) => {
    if (!selectedGuildId) return
    try {
      await kickMember(selectedGuildId, pub)
      api.listMembers(selectedGuildId).then(setMembers)
    } catch (e: any) {
      alert('Kick failed: ' + e.message)
    }
  }

  const handleStartDM = (pub: string) => {
    useGuildStore.setState({ selectedGuildId: null })
    useDMStore.getState().selectPeer(pub)
    if (!useDMStore.getState().conversations.find(c => c.peerPub === pub)) {
      useDMStore.setState(s => ({
        conversations: [...s.conversations, { peerPub: pub }],
      }))
    }
  }

  const displayName = (pub: string) => {
    return users[pub] || pub.slice(0, 12) + '...'
  }

  return (
    <div className="w-60 bg-bg-secondary shrink-0 flex flex-col border-l border-border-thin">
      <div className="h-12 px-4 flex items-center justify-between border-b border-border-thin shadow-sm">
        <span className="text-xs font-semibold text-text-muted uppercase tracking-wide">
          Members — {members.length}
        </span>
        {isOwner && (
          <button
            onClick={() => setShowInvite(!showInvite)}
            className="text-text-muted hover:text-text-primary text-lg leading-none"
            title="Invite Member"
          >
            +
          </button>
        )}
      </div>

      {showInvite && (
        <div className="px-3 py-3 border-b border-border-thin">
          <label className="text-xs text-text-muted uppercase font-semibold tracking-wide mb-1.5 block">
            Invite by public key
          </label>
          <input
            className="w-full bg-bg-input text-text-primary rounded px-3 py-2 text-xs mb-2"
            placeholder="Paste ed25519 public key..."
            value={invitePub}
            onChange={e => setInvitePub(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') handleInvite() }}
            autoFocus
          />
          <button
            onClick={handleInvite}
            className="w-full text-xs bg-bg-accent text-white rounded py-1.5 font-semibold hover:bg-bg-accent-hover transition-colors"
          >
            Send Invite
          </button>
        </div>
      )}

      <div className="flex-1 overflow-y-auto px-2 py-2">
        {members.map(pub => {
          const isMe = pub === myPub
          const name = displayName(pub)
          const isMemberOwner = pub === guild?.owner_pub

          return (
            <div
              key={pub}
              className="flex items-center gap-3 px-2 py-1.5 rounded hover:bg-bg-message-hover group cursor-pointer"
              onClick={() => !isMe && handleStartDM(pub)}
              title={isMe ? undefined : 'Click to send a DM'}
            >
              <Avatar url={selectedGuildId ? getAvatarUrl(selectedGuildId, pub) : null} letter={name} size="md" />
              <div className="flex-1 min-w-0">
                <div className="text-sm text-text-primary truncate">
                  {name}
                  {isMemberOwner && <span className="text-text-muted text-[10px] ml-1.5">OWNER</span>}
                </div>
              </div>
              {!isMe && (
                <div className="hidden group-hover:flex items-center gap-1">
                  {isOwner && !isMemberOwner && (
                    <button
                      onClick={(e) => { e.stopPropagation(); handleKick(pub) }}
                      className="text-danger text-[10px] font-semibold hover:underline px-1"
                    >
                      Kick
                    </button>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

import { useState } from 'react'
import { useGuildStore } from '../stores/guilds'
import { useMessageStore } from '../stores/messages'

export default function MessageInput() {
  const { selectedGuildId, selectedChannelId, channels } = useGuildStore()
  const { sendMessage } = useMessageStore()
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)

  const channelName = selectedGuildId
    ? channels[selectedGuildId]?.find(c => c.id === selectedChannelId)?.name || 'general'
    : ''

  if (!selectedGuildId || !selectedChannelId) return null

  const handleSend = async () => {
    if (!text.trim() || sending) return
    setSending(true)
    try {
      await sendMessage(selectedGuildId, selectedChannelId, text.trim())
      setText('')
    } catch (e: any) {
      console.error('send error:', e)
    } finally {
      setSending(false)
    }
  }

  return (
    <div className="px-4 pb-6 shrink-0">
      <div className="bg-bg-input rounded-lg flex items-center px-4">
        <input
          className="flex-1 bg-transparent text-text-primary py-3 text-sm placeholder:text-text-muted"
          placeholder={`Message #${channelName}`}
          value={text}
          onChange={e => setText(e.target.value)}
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
  )
}

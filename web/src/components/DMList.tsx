import { useDMStore } from '../stores/dms'

export default function DMList() {
  const { conversations, selectedPeer, selectPeer } = useDMStore()

  return (
    <div className="px-2 mt-4">
      <div className="flex items-center justify-between px-1 mb-1">
        <span className="text-xs font-semibold text-text-muted uppercase tracking-wide">
          Direct Messages
        </span>
      </div>
      {conversations.length === 0 ? (
        <div className="text-text-muted text-xs px-2 py-2">No DMs yet</div>
      ) : (
        conversations.map(c => (
          <button
            key={c.peerPub}
            onClick={() => selectPeer(c.peerPub)}
            className={`w-full text-left px-2 py-1.5 rounded text-sm flex items-center gap-2
              ${selectedPeer === c.peerPub ? 'bg-bg-message-hover text-white' : 'text-channel-default hover:text-channel-hover hover:bg-bg-message-hover'}`}
          >
            <span className="w-6 h-6 rounded-full bg-bg-accent flex items-center justify-center text-[10px] font-bold text-white shrink-0">
              {c.peerPub.charAt(0).toUpperCase()}
            </span>
            <span className="truncate text-xs">{c.peerName || c.peerPub.slice(0, 16) + '...'}</span>
          </button>
        ))
      )}
    </div>
  )
}

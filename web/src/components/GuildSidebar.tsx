import { useGuildStore } from '../stores/guilds'
import { useState } from 'react'

export default function GuildSidebar() {
  const { guilds, selectedGuildId, selectGuild, createGuild } = useGuildStore()
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState('')

  const handleCreate = async () => {
    if (!newName.trim()) return
    const g = await createGuild(newName.trim())
    selectGuild(g.id)
    setNewName('')
    setShowCreate(false)
  }

  return (
    <div className="w-[72px] bg-bg-tertiary flex flex-col items-center py-3 gap-2 shrink-0 overflow-y-auto">
      {/* Home / DM button */}
      <button
        onClick={() => useGuildStore.setState({ selectedGuildId: null })}
        className={`w-12 h-12 rounded-2xl flex items-center justify-center font-bold text-lg transition-all cursor-pointer
          ${!selectedGuildId ? 'bg-bg-accent text-white rounded-xl' : 'bg-bg-primary text-text-primary hover:rounded-xl hover:bg-bg-accent'}`}
        title="Direct Messages"
      >
        H
      </button>

      <div className="w-8 h-0.5 bg-bg-primary rounded-full my-1" />

      {guilds.map(g => (
        <button
          key={g.id}
          onClick={() => selectGuild(g.id)}
          className={`w-12 h-12 rounded-2xl flex items-center justify-center font-semibold text-sm transition-all cursor-pointer
            ${selectedGuildId === g.id ? 'bg-bg-accent text-white rounded-xl' : 'bg-bg-primary text-text-primary hover:rounded-xl hover:bg-bg-accent'}`}
          title={g.name}
        >
          {g.name.charAt(0).toUpperCase()}
        </button>
      ))}

      {/* Create guild */}
      {showCreate ? (
        <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50 p-4" onClick={() => setShowCreate(false)}>
          <div className="bg-bg-primary rounded-xl px-8 py-6 w-full max-w-md shadow-2xl" onClick={e => e.stopPropagation()}>
            <h2 className="text-white text-xl font-bold mb-1">Create a Guild</h2>
            <p className="text-text-secondary text-sm mb-5">Your guild is where you and your friends hang out.</p>
            <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wide mb-2">
              Guild Name
            </label>
            <input
              className="w-full bg-bg-input text-text-primary rounded-md px-4 py-2.5 mb-6 text-sm"
              placeholder="e.g. My Cool Server"
              value={newName}
              onChange={e => setNewName(e.target.value)}
              onKeyDown={e => e.key === 'Enter' && handleCreate()}
              autoFocus
            />
            <div className="flex justify-end gap-3">
              <button
                onClick={() => setShowCreate(false)}
                className="px-4 py-2 text-sm text-text-muted hover:text-text-primary transition-colors"
              >
                Cancel
              </button>
              <button
                onClick={handleCreate}
                className="px-5 py-2 text-sm bg-bg-accent text-white rounded-md font-semibold hover:bg-bg-accent-hover transition-colors"
              >
                Create Guild
              </button>
            </div>
          </div>
        </div>
      ) : null}

      <button
        onClick={() => setShowCreate(true)}
        className="w-12 h-12 rounded-2xl bg-bg-primary text-online flex items-center justify-center text-2xl hover:rounded-xl hover:bg-online hover:text-white transition-all cursor-pointer"
        title="Create Guild"
      >
        +
      </button>
    </div>
  )
}

import { useState } from 'react'
import { useAuthStore } from '../stores/auth'

export default function Unlock() {
  const { unlock, username } = useAuthStore()
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      await unlock(password)
    } catch {
      setError('Incorrect password or corrupted key')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-bg-tertiary flex items-center justify-center p-4">
      <form onSubmit={handleSubmit} className="bg-bg-primary rounded-xl px-10 py-8 w-full max-w-md shadow-2xl">
        <h1 className="text-2xl font-bold text-white mb-2 text-center">Welcome back, {username}</h1>
        <p className="text-text-secondary text-sm mb-8 text-center leading-relaxed">
          Enter your encryption password to unlock your private key.
        </p>

        <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wide mb-2">
          Encryption Password
        </label>
        <input
          type="password"
          className="w-full bg-bg-input text-text-primary rounded-md px-4 py-2.5 mb-6 text-sm"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="Enter your password"
          autoFocus
        />

        {error && <p className="text-danger text-sm mb-4">{error}</p>}

        <button
          type="submit"
          disabled={loading}
          className="w-full bg-bg-accent text-white rounded-md py-2.5 font-semibold hover:bg-bg-accent-hover disabled:opacity-50 transition-colors"
        >
          {loading ? 'Decrypting...' : 'Unlock'}
        </button>
      </form>
    </div>
  )
}

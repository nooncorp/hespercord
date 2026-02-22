import { useState } from 'react'
import { useAuthStore } from '../stores/auth'
import { useNavigate } from 'react-router-dom'

export default function Signup() {
  const { signup } = useAuthStore()
  const navigate = useNavigate()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (password.length < 8) { setError('Password must be at least 8 characters'); return }
    if (password !== confirm) { setError('Passwords do not match'); return }
    setLoading(true)
    setError('')
    try {
      await signup(password)
      navigate('/app')
    } catch (e: any) {
      setError(e.message)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen bg-bg-tertiary flex items-center justify-center p-4">
      <form onSubmit={handleSubmit} className="bg-bg-primary rounded-xl px-10 py-8 w-full max-w-md shadow-2xl">
        <h1 className="text-2xl font-bold text-white mb-2 text-center">Welcome to Hespercord</h1>
        <p className="text-text-secondary text-sm mb-8 text-center leading-relaxed">
          Set an encryption password to protect your private key.
          This password never leaves your browser.
        </p>

        <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wide mb-2">
          Encryption Password
        </label>
        <input
          type="password"
          className="w-full bg-bg-input text-text-primary rounded-md px-4 py-2.5 mb-5 text-sm"
          value={password}
          onChange={e => setPassword(e.target.value)}
          placeholder="At least 8 characters"
          autoFocus
        />

        <label className="block text-xs font-semibold text-text-secondary uppercase tracking-wide mb-2">
          Confirm Password
        </label>
        <input
          type="password"
          className="w-full bg-bg-input text-text-primary rounded-md px-4 py-2.5 mb-6 text-sm"
          value={confirm}
          onChange={e => setConfirm(e.target.value)}
          placeholder="Re-enter password"
        />

        {error && <p className="text-danger text-sm mb-4">{error}</p>}

        <button
          type="submit"
          disabled={loading}
          className="w-full bg-bg-accent text-white rounded-md py-2.5 font-semibold hover:bg-bg-accent-hover disabled:opacity-50 transition-colors"
        >
          {loading ? 'Generating keys...' : 'Create Identity'}
        </button>

        <p className="text-text-muted text-xs mt-6 text-center leading-relaxed">
          Your Ed25519 keypair will be generated in your browser.
          The private key is encrypted with your password before being stored on the server.
        </p>
      </form>
    </div>
  )
}

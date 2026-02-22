import { useAuthStore } from '../stores/auth'

export default function KeyDownload() {
  const { identity, downloadKeypair } = useAuthStore()

  if (!identity) return null

  return (
    <button
      onClick={downloadKeypair}
      className="text-xs text-text-muted hover:text-text-primary underline"
      title="Download your Ed25519/X25519 keypair as JSON"
    >
      Download Keypair
    </button>
  )
}

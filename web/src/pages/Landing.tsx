export default function Landing() {
  return (
    <div className="min-h-screen bg-bg-tertiary flex flex-col">
      <nav className="flex items-center justify-between px-8 py-4">
        <span className="text-xl font-bold text-white">Hespercord</span>
        <a
          href="/auth/discord"
          className="rounded bg-bg-accent px-4 py-2 text-sm font-medium text-white hover:bg-bg-accent-hover transition-colors"
        >
          Sign in with Discord
        </a>
      </nav>

      <main className="flex-1 flex flex-col items-center justify-center px-4 text-center">
        <h1 className="text-5xl font-extrabold text-white mb-6">
          Encrypted group chat,
          <br />
          <span className="text-bg-accent">Discord-style</span>
        </h1>
        <p className="max-w-xl text-text-secondary text-lg mb-8 leading-relaxed">
          Hespercord combines the familiar UX of Discord with end-to-end
          encryption. Guild messages are encrypted with a shared guild key
          (XChaCha20-Poly1305). Direct messages use the Signal Protocol.
          The server never sees your plaintext.
        </p>

        <div className="grid grid-cols-1 md:grid-cols-3 gap-6 max-w-3xl w-full mb-12">
          <FeatureCard
            title="Guild E2EE"
            desc="Messages encrypted at rest with a per-guild symmetric key. The relay stores only ciphertext."
          />
          <FeatureCard
            title="Signal DMs"
            desc="1:1 conversations use the full Signal Protocol — X3DH key agreement + Double Ratchet."
          />
          <FeatureCard
            title="Your Keys"
            desc="Ed25519 identity keypair, derived X25519 for key agreement. Download and control your own keys."
          />
        </div>

        <a
          href="/auth/discord"
          className="rounded-full bg-bg-accent px-8 py-3 text-lg font-semibold text-white hover:bg-bg-accent-hover transition-colors"
        >
          Get Started
        </a>
      </main>

      <footer className="text-text-muted text-xs text-center py-4">
        Hespercord &mdash; open protocol, encrypted by default
      </footer>
    </div>
  )
}

function FeatureCard({ title, desc }: { title: string; desc: string }) {
  return (
    <div className="rounded-lg bg-bg-secondary p-6 text-left">
      <h3 className="text-white font-semibold mb-2">{title}</h3>
      <p className="text-text-secondary text-sm leading-relaxed">{desc}</p>
    </div>
  )
}

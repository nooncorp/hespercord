import { useEffect } from 'react'
import { Routes, Route, Navigate } from 'react-router-dom'
import { useAuthStore } from '../stores/auth'
import { useGuildStore } from '../stores/guilds'
import { useDMStore } from '../stores/dms'
import { useMessageStore } from '../stores/messages'
import { wsClient } from '../lib/ws'
import GuildSidebar from '../components/GuildSidebar'
import ChannelList from '../components/ChannelList'
import MessageArea from '../components/MessageArea'
import MessageInput from '../components/MessageInput'
import MemberList from '../components/MemberList'
import DMArea from '../components/DMArea'
import Signup from './Signup'
import Unlock from './Unlock'

export default function AppShell() {
  const { loading, authenticated, isNew, identity, checkAuth } = useAuthStore()
  const { loadGuilds } = useGuildStore()
  const { loadConversations } = useDMStore()

  useEffect(() => {
    checkAuth()
  }, [])

  useEffect(() => {
    if (authenticated && identity) {
      loadGuilds()
      loadConversations()
      wsClient.connect()

      wsClient.on('guild_message', (msg: any) => {
        useMessageStore.getState().addMessage(msg.guild_id, msg.envelope)
      })

      wsClient.on('dm_message', (msg: any) => {
        useDMStore.getState().addIncomingDM(msg.message)
      })

      wsClient.on('guild_added', () => {
        useGuildStore.getState().loadGuilds()
      })

      return () => wsClient.disconnect()
    }
  }, [authenticated, identity])

  if (loading) {
    return (
      <div className="h-screen flex items-center justify-center bg-bg-tertiary">
        <div className="text-text-muted">Loading...</div>
      </div>
    )
  }

  if (!authenticated) {
    return (
      <div className="h-screen flex items-center justify-center bg-bg-tertiary">
        <div className="bg-bg-primary rounded-xl px-10 py-8 w-full max-w-sm shadow-2xl text-center">
          <h1 className="text-2xl font-bold text-white mb-2">Hespercord</h1>
          <p className="text-text-secondary text-sm mb-8">Sign in to continue</p>
          <a
            href="/auth/discord"
            className="block rounded-md bg-bg-accent px-6 py-2.5 text-white font-semibold hover:bg-bg-accent-hover transition-colors"
          >
            Sign in with Discord
          </a>
        </div>
      </div>
    )
  }

  return (
    <Routes>
      <Route path="signup" element={isNew ? <Signup /> : <Navigate to="/app" />} />
      <Route path="*" element={
        isNew ? <Navigate to="/app/signup" /> :
        !identity ? <Unlock /> :
        <MainApp />
      } />
    </Routes>
  )
}

function MainApp() {
  const selectedGuildId = useGuildStore(s => s.selectedGuildId)

  return (
    <div className="h-screen flex bg-bg-primary text-text-primary overflow-hidden">
      <GuildSidebar />
      <ChannelList />
      {selectedGuildId ? (
        <>
          <div className="flex-1 flex flex-col min-w-0 min-h-0">
            <MessageArea />
            <MessageInput />
          </div>
          <MemberList />
        </>
      ) : (
        <DMArea />
      )}
    </div>
  )
}

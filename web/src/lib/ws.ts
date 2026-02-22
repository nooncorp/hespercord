type WSHandler = (msg: any) => void

class WebSocketClient {
  private ws: WebSocket | null = null
  private handlers: Map<string, WSHandler[]> = new Map()
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private subscribedGuilds: Set<string> = new Set()

  connect() {
    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.ws = new WebSocket(`${proto}//${location.host}/ws`)

    this.ws.onopen = () => {
      for (const guildId of this.subscribedGuilds) {
        this.send({ type: 'subscribe', guild_id: guildId })
      }
    }

    this.ws.onmessage = (event) => {
      try {
        const msg = JSON.parse(event.data)
        const handlers = this.handlers.get(msg.type) || []
        for (const h of handlers) h(msg)
        const allHandlers = this.handlers.get('*') || []
        for (const h of allHandlers) h(msg)
      } catch { /* ignore malformed */ }
    }

    this.ws.onclose = () => {
      this.reconnectTimer = setTimeout(() => this.connect(), 3000)
    }

    this.ws.onerror = () => {
      this.ws?.close()
    }
  }

  disconnect() {
    if (this.reconnectTimer) clearTimeout(this.reconnectTimer)
    this.ws?.close()
    this.ws = null
  }

  subscribe(guildId: string) {
    this.subscribedGuilds.add(guildId)
    this.send({ type: 'subscribe', guild_id: guildId })
  }

  unsubscribe(guildId: string) {
    this.subscribedGuilds.delete(guildId)
    this.send({ type: 'unsubscribe', guild_id: guildId })
  }

  on(type: string, handler: WSHandler) {
    if (!this.handlers.has(type)) this.handlers.set(type, [])
    this.handlers.get(type)!.push(handler)
  }

  off(type: string, handler: WSHandler) {
    const list = this.handlers.get(type)
    if (list) {
      this.handlers.set(type, list.filter(h => h !== handler))
    }
  }

  private send(data: any) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data))
    }
  }
}

export const wsClient = new WebSocketClient()

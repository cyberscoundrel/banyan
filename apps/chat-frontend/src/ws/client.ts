type EventHandler = (event: WsEvent) => void
type ConnectionHandler = () => void

export interface WsEvent {
  event: string
  data: Record<string, unknown>
  timestamp?: number
}

class WebSocketClient {
  private ws: WebSocket | null = null
  private reconnectAttempts = 0
  private maxReconnectAttempts = 5
  private reconnectDelay = 1000
  private handlers: EventHandler[] = []
  private connectHandlers: ConnectionHandler[] = []
  private disconnectHandlers: ConnectionHandler[] = []

  connect() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ledger/events`

    try {
      this.ws = new WebSocket(wsUrl)
      this.ws.onopen = () => {
        console.log('WebSocket connected')
        this.reconnectAttempts = 0
        this.subscribe(['channel:new', 'post:new', 'post:delete', 'sync:entry'])
        this.connectHandlers.forEach((h) => h())
      }
      this.ws.onclose = () => {
        console.log('WebSocket disconnected')
        this.disconnectHandlers.forEach((h) => h())
        this.attemptReconnect()
      }
      this.ws.onerror = (err) => {
        console.error('WebSocket error:', err)
      }
      this.ws.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data) as WsEvent
          this.handlers.forEach((h) => h(data))
        } catch (err) {
          console.error('Failed to parse WebSocket message:', err)
        }
      }
    } catch (err) {
      console.error('Failed to create WebSocket:', err)
      this.attemptReconnect()
    }
  }

  private subscribe(events: string[]) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type: 'subscribe', events }))
    }
  }

  private attemptReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnect attempts reached')
      return
    }
    this.reconnectAttempts++
    const delay = this.reconnectDelay * Math.pow(2, this.reconnectAttempts - 1)
    console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`)
    setTimeout(() => this.connect(), delay)
  }

  disconnect() {
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
  }

  onMessage(handler: EventHandler) {
    this.handlers.push(handler)
  }

  onConnect(handler: ConnectionHandler) {
    this.connectHandlers.push(handler)
  }

  onDisconnect(handler: ConnectionHandler) {
    this.disconnectHandlers.push(handler)
  }
}

export const wsClient = new WebSocketClient()

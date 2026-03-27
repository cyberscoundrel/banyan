type EventHandler = (event: WsEvent) => void
type ConnectionHandler = () => void

export interface WsEvent {
  type: 'new_post' | 'delete_post' | 'channel_update' | 'user_join' | 'user_leave'
  data: Record<string, unknown>
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
    const wsUrl = `${protocol}//${window.location.host}/events`

    try {
      this.ws = new WebSocket(wsUrl)
      this.ws.onopen = () => {
        console.log('WebSocket connected')
        this.reconnectAttempts = 0
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

  send(data: unknown) {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data))
    }
  }
}

export const wsClient = new WebSocketClient()

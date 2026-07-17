const API_BASE = ''

export interface Channel {
  name: string
  description?: string
  createdAt: number
  createdBy: string
}

export interface Message {
  id: string
  channel: string
  author: string
  content: string
  timestamp: number
  replyTo?: string
  deleted: boolean
}

class ApiClient {
  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>),
    }

    const response = await fetch(`${API_BASE}${path}`, {
      ...options,
      headers,
    })

    if (!response.ok) {
      const error = await response.json().catch(() => ({ message: 'Request failed' }))
      throw new Error(error.message || `HTTP ${response.status}`)
    }

    return response.json()
  }

  async getChannels(): Promise<Channel[]> {
    const data = await this.request<{ channels: Channel[] }>('/ledger/channels')
    return data.channels ?? []
  }

  async createChannel(name: string, description?: string): Promise<{ channel: Channel }> {
    return this.request<{ channel: Channel }>('/ledger/channels', {
      method: 'POST',
      body: JSON.stringify({ name, description }),
    })
  }

  async getPosts(channel: string, limit = 50): Promise<Message[]> {
    const params = new URLSearchParams({ channel, limit: String(limit) })
    const data = await this.request<{ posts: Message[] }>(`/ledger/posts?${params}`)
    return data.posts ?? []
  }

  async createPost(channel: string, content: string): Promise<{ entry: any }> {
    return this.request<{ entry: any }>('/ledger/post', {
      method: 'POST',
      body: JSON.stringify({ channel, content }),
    })
  }

  async deletePost(postId: string): Promise<void> {
    await this.request('/ledger/delete', {
      method: 'POST',
      body: JSON.stringify({ postId }),
    })
  }

  async getPeerInfo(): Promise<{ peerId: string }> {
    return this.request<{ peerId: string }>('/peer-info')
  }
}

export const apiClient = new ApiClient()

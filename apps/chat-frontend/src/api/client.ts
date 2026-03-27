const API_BASE = '/api'

export interface Channel {
  id: string
  name: string
  description?: string
}

export interface Message {
  id: string
  channelId: string
  author: string
  content: string
  timestamp: number
}

export interface ChallengeResponse {
  challenge: string
  expiresAt: number
}

export interface VerifyResponse {
  peerId: string
  token?: string
}

class ApiClient {
  private token: string | null = null

  setToken(token: string) {
    this.token = token
  }

  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>),
    }

    if (this.token) {
      headers['Authorization'] = `Bearer ${this.token}`
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

  async getChallenge(): Promise<ChallengeResponse> {
    return this.request<ChallengeResponse>('/auth/challenge')
  }

  async verifyChallenge(challenge: string, signature: string): Promise<VerifyResponse> {
    const response = await this.request<VerifyResponse & { token?: string }>('/auth/verify', {
      method: 'POST',
      body: JSON.stringify({ challenge, signature }),
    })
    if (response.token) {
      this.token = response.token
    }
    return response
  }

  async getChannels(): Promise<Channel[]> {
    return this.request<Channel[]>('/channels')
  }

  async getPosts(channelId: string, limit = 50, before?: string): Promise<Message[]> {
    const params = new URLSearchParams({ limit: String(limit) })
    if (before) params.set('before', before)
    return this.request<Message[]>(`/posts?channelId=${channelId}&${params}`)
  }

  async createPost(channelId: string, content: string): Promise<Message> {
    return this.request<Message>('/ledger/post', {
      method: 'POST',
      body: JSON.stringify({ channelId, content }),
    })
  }

  async deletePost(postId: string, reason?: string): Promise<void> {
    await this.request('/ledger/delete', {
      method: 'POST',
      body: JSON.stringify({ targetId: postId, reason }),
    })
  }
}

export const apiClient = new ApiClient()

import { useState, useEffect, useCallback } from 'react'
import { Auth } from './components/Auth'
import { ChannelList } from './components/ChannelList'
import { MessageList } from './components/MessageList'
import { PostComposer } from './components/PostComposer'
import { apiClient, Channel, Message } from './api/client'
import { wsClient } from './ws/client'

export interface User {
  peerId: string
  authenticated: boolean
}

export function App() {
  const [user, setUser] = useState<User | null>(null)
  const [channels, setChannels] = useState<Channel[]>([])
  const [selectedChannel, setSelectedChannel] = useState<string | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [connected, setConnected] = useState(false)

  useEffect(() => {
    loadChannels()
  }, [])

  useEffect(() => {
    if (selectedChannel) {
      loadMessages(selectedChannel)
    }
  }, [selectedChannel])

  useEffect(() => {
    wsClient.onConnect(() => setConnected(true))
    wsClient.onDisconnect(() => setConnected(false))
    wsClient.onMessage((event) => {
      if (event.type === 'new_post' && event.data.channelId === selectedChannel) {
        setMessages((prev) => [...prev, event.data])
      } else if (event.type === 'delete_post') {
        setMessages((prev) => prev.filter((m) => m.id !== event.data.postId))
      }
    })
    wsClient.connect()
    return () => wsClient.disconnect()
  }, [selectedChannel])

  const loadChannels = async () => {
    try {
      const data = await apiClient.getChannels()
      setChannels(data)
      if (data.length > 0 && !selectedChannel) {
        setSelectedChannel(data[0].id)
      }
    } catch (err) {
      console.error('Failed to load channels:', err)
    }
  }

  const loadMessages = async (channelId: string) => {
    try {
      const data = await apiClient.getPosts(channelId)
      setMessages(data)
    } catch (err) {
      console.error('Failed to load messages:', err)
    }
  }

  const handleAuth = useCallback((peerId: string) => {
    setUser({ peerId, authenticated: true })
  }, [])

  const handlePost = async (content: string) => {
    if (!selectedChannel || !user) return
    try {
      await apiClient.createPost(selectedChannel, content)
    } catch (err) {
      console.error('Failed to post:', err)
    }
  }

  if (!user) {
    return <Auth onAuth={handleAuth} />
  }

  return (
    <div className="flex h-screen">
      <div className="w-64 bg-gray-800 border-r border-gray-700">
        <div className="p-4 border-b border-gray-700">
          <div className="flex items-center gap-2">
            <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-sm text-gray-400">Connected</span>
          </div>
          <p className="text-xs text-gray-500 mt-1 truncate" title={user.peerId}>
            {user.peerId.slice(0, 20)}...
          </p>
        </div>
        <ChannelList
          channels={channels}
          selectedId={selectedChannel}
          onSelect={setSelectedChannel}
        />
      </div>
      <div className="flex-1 flex flex-col">
        {selectedChannel ? (
          <>
            <div className="flex-1 overflow-y-auto">
              <MessageList messages={messages} currentPeerId={user.peerId} />
            </div>
            <div className="border-t border-gray-700 p-4">
              <PostComposer onPost={handlePost} />
            </div>
          </>
        ) : (
          <div className="flex-1 flex items-center justify-center text-gray-500">
            Select a channel to start chatting
          </div>
        )}
      </div>
    </div>
  )
}

import { useState, useEffect } from 'react'
import { ChannelList } from './components/ChannelList'
import { MessageList } from './components/MessageList'
import { PostComposer } from './components/PostComposer'
import { apiClient, Channel, Message } from './api/client'
import { wsClient } from './ws/client'

export function App() {
  const [channels, setChannels] = useState<Channel[]>([])
  const [selectedChannel, setSelectedChannel] = useState<string | null>(null)
  const [messages, setMessages] = useState<Message[]>([])
  const [connected, setConnected] = useState(false)
  const [peerId, setPeerId] = useState<string>('')

  useEffect(() => {
    loadPeerInfo()
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
      if (event.event === 'post:new') {
        const raw = (event.data as any).post || (event.data as any).entry
        if (!raw) return
        const channel = raw.channel || raw.data?.channel
        const content = raw.content || raw.data?.content
        const replyTo = raw.replyTo || raw.data?.replyTo
        if (channel === selectedChannel) {
          setMessages((prev) => {
            if (prev.some((m) => m.id === raw.id)) return prev
            return [...prev, {
              id: raw.id,
              channel,
              author: raw.author,
              content,
              timestamp: raw.timestamp,
              replyTo,
              deleted: false,
            }]
          })
        }
      } else if (event.event === 'post:delete') {
        const { postId } = event.data as any
        if (postId) {
          setMessages((prev) => prev.filter((m) => m.id !== postId))
        }
      } else if (event.event === 'channel:new') {
        const { channel } = event.data as any
        if (channel) {
          setChannels((prev) => {
            if (prev.some((c) => c.name === channel.name)) return prev
            return [...prev, channel as Channel].sort((a, b) => a.name.localeCompare(b.name))
          })
        }
      }
    })
    wsClient.connect()
    return () => wsClient.disconnect()
  }, [selectedChannel])

  const loadPeerInfo = async () => {
    try {
      const info = await apiClient.getPeerInfo()
      setPeerId(info.peerId)
    } catch {
      // Not proxied through Banyan, that's ok
    }
  }

  const loadChannels = async () => {
    try {
      const channels = await apiClient.getChannels()
      setChannels(channels)
      if (channels.length > 0 && !selectedChannel) {
        setSelectedChannel(channels[0].name)
      }
    } catch (err) {
      console.error('Failed to load channels:', err)
    }
  }

  const loadMessages = async (channelName: string) => {
    try {
      const msgs = await apiClient.getPosts(channelName)
      setMessages(msgs)
    } catch (err) {
      console.error('Failed to load messages:', err)
    }
  }

  const handlePost = async (content: string) => {
    if (!selectedChannel) return
    try {
      const { entry } = await apiClient.createPost(selectedChannel, content)
      const d = entry.data as any
      setMessages((prev) => {
        if (prev.some((m) => m.id === entry.id)) return prev
        return [...prev, {
          id: entry.id,
          channel: d.channel || selectedChannel,
          author: entry.author || peerId,
          content: d.content || content,
          timestamp: entry.timestamp,
          replyTo: d.replyTo,
          deleted: false,
        }]
      })
    } catch (err) {
      console.error('Failed to post:', err)
    }
  }

  const handleCreateChannel = async (name: string, description?: string) => {
    try {
      await apiClient.createChannel(name, description)
      await loadChannels()
    } catch (err) {
      console.error('Failed to create channel:', err)
    }
  }

  return (
    <div className="flex h-screen">
      <div className="w-64 bg-gray-800 border-r border-gray-700">
        <div className="p-4 border-b border-gray-700">
          <div className="flex items-center gap-2">
            <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
            <span className="text-sm text-gray-400">{connected ? 'Connected' : 'Disconnected'}</span>
          </div>
          {peerId && (
            <div className="mt-1 text-xs text-gray-600 font-mono truncate" title={peerId}>
              {peerId.slice(0, 12)}...
            </div>
          )}
        </div>
        <ChannelList
          channels={channels}
          selectedChannel={selectedChannel}
          onSelect={setSelectedChannel}
          onCreateChannel={handleCreateChannel}
        />
      </div>
      <div className="flex-1 flex flex-col">
        {selectedChannel ? (
          <>
            <div className="flex-1 overflow-y-auto">
              <MessageList messages={messages} currentPeerId={peerId} />
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

import { Message } from '../api/client'

interface MessageListProps {
  messages: Message[]
  currentPeerId: string
}

function formatTime(timestamp: number): string {
  const date = new Date(timestamp / 1000000)
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function shortenPeerId(peerId: string): string {
  return peerId.slice(0, 8) + '...' + peerId.slice(-4)
}

export function MessageList({ messages, currentPeerId }: MessageListProps) {
  if (messages.length === 0) {
    return (
      <div className="flex items-center justify-center h-full text-gray-500">
        No messages yet. Be the first to post!
      </div>
    )
  }

  return (
    <div className="p-4 space-y-4">
      {messages.map((message) => {
        const isOwn = message.author === currentPeerId
        return (
          <div key={message.id} className={`flex ${isOwn ? 'justify-end' : 'justify-start'}`}>
            <div className={`max-w-[70%] ${isOwn ? 'order-1' : ''}`}>
              <div className="flex items-center gap-2 mb-1">
                {!isOwn && (
                  <span className="text-xs text-blue-400 font-mono">
                    {shortenPeerId(message.author)}
                  </span>
                )}
                <span className="text-xs text-gray-500">
                  {formatTime(message.timestamp)}
                </span>
              </div>
              <div
                className={`px-4 py-2 rounded-lg ${
                  isOwn
                    ? 'bg-blue-600 text-white'
                    : 'bg-gray-700 text-gray-100'
                }`}
              >
                <p className="whitespace-pre-wrap break-words">{message.content}</p>
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}

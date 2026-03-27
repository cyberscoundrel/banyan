import { Channel } from '../api/client'

interface ChannelListProps {
  channels: Channel[]
  selectedId: string | null
  onSelect: (id: string) => void
}

export function ChannelList({ channels, selectedId, onSelect }: ChannelListProps) {
  if (channels.length === 0) {
    return (
      <div className="p-4 text-gray-500 text-sm">
        No channels available
      </div>
    )
  }

  return (
    <div className="p-2">
      <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wider px-2 mb-2">
        Channels
      </h2>
      <ul className="space-y-1">
        {channels.map((channel) => (
          <li key={channel.id}>
            <button
              onClick={() => onSelect(channel.id)}
              className={`w-full text-left px-3 py-2 rounded transition-colors ${
                selectedId === channel.id
                  ? 'bg-gray-700 text-white'
                  : 'text-gray-300 hover:bg-gray-700/50'
              }`}
            >
              <span className="text-gray-400 mr-2">#</span>
              {channel.name}
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}

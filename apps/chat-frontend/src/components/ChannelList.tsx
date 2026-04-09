import { useState } from 'react'
import { Channel } from '../api/client'

interface ChannelListProps {
  channels: Channel[]
  selectedChannel: string | null
  onSelect: (name: string) => void
  onCreateChannel: (name: string, description?: string) => Promise<void>
}

export function ChannelList({ channels, selectedChannel, onSelect, onCreateChannel }: ChannelListProps) {
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [error, setError] = useState('')
  const [creating, setCreating] = useState(false)

  const handleCreate = async () => {
    if (!name.trim()) return
    setCreating(true)
    setError('')
    try {
      await onCreateChannel(name.trim(), description.trim() || undefined)
      setName('')
      setDescription('')
      setShowForm(false)
    } catch (err: any) {
      setError(err.message || 'Failed to create channel')
    } finally {
      setCreating(false)
    }
  }

  return (
    <div className="p-2">
      <div className="flex items-center justify-between px-2 mb-2">
        <h2 className="text-xs font-semibold text-gray-500 uppercase tracking-wider">
          Channels
        </h2>
        <button
          onClick={() => setShowForm(!showForm)}
          className="text-xs text-gray-400 hover:text-white transition-colors"
        >
          {showForm ? 'Cancel' : '+ New'}
        </button>
      </div>

      {showForm && (
        <div className="px-2 mb-3 space-y-2">
          {error && (
            <p className="text-xs text-red-400">{error}</p>
          )}
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Channel name"
            className="w-full bg-gray-700 text-white text-sm rounded px-3 py-1.5 placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-gray-500"
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            autoFocus
          />
          <input
            type="text"
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            placeholder="Description (optional)"
            className="w-full bg-gray-700 text-white text-sm rounded px-3 py-1.5 placeholder-gray-400 focus:outline-none focus:ring-1 focus:ring-gray-500"
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
          />
          <button
            onClick={handleCreate}
            disabled={creating || !name.trim()}
            className="w-full bg-gray-600 hover:bg-gray-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm rounded px-3 py-1.5 transition-colors"
          >
            {creating ? 'Creating...' : 'Create'}
          </button>
        </div>
      )}

      {channels.length === 0 && !showForm ? (
        <div className="px-2 text-gray-500 text-sm">
          No channels yet
        </div>
      ) : (
        <ul className="space-y-1">
          {channels.map((channel) => (
            <li key={channel.name}>
              <button
                onClick={() => onSelect(channel.name)}
                className={`w-full text-left px-3 py-2 rounded transition-colors ${
                  selectedChannel === channel.name
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
      )}
    </div>
  )
}

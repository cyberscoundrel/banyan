import { useState } from 'react'
import { apiClient } from '../api/client'

interface AuthProps {
  onAuth: (peerId: string) => void
}

export function Auth({ onAuth }: AuthProps) {
  const [challenge, setChallenge] = useState<string | null>(null)
  const [signature, setSignature] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const requestChallenge = async () => {
    setLoading(true)
    setError(null)
    try {
      const data = await apiClient.getChallenge()
      setChallenge(data.challenge)
    } catch (err) {
      setError('Failed to get challenge')
    } finally {
      setLoading(false)
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!challenge || !signature) return

    setLoading(true)
    setError(null)
    try {
      const data = await apiClient.verifyChallenge(challenge, signature)
      onAuth(data.peerId)
    } catch (err) {
      setError('Authentication failed')
    } finally {
      setLoading(false)
    }
  }

  if (!challenge) {
    return (
      <div className="min-h-screen flex items-center justify-center bg-gray-900">
        <div className="bg-gray-800 p-8 rounded-lg shadow-xl max-w-md w-full">
          <h1 className="text-2xl font-bold text-center mb-6">Banyan Chat</h1>
          <p className="text-gray-400 text-center mb-6">
            Authenticate using your libp2p peer ID
          </p>
          <button
            onClick={requestChallenge}
            disabled={loading}
            className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white py-2 px-4 rounded transition-colors"
          >
            {loading ? 'Loading...' : 'Get Challenge'}
          </button>
          {error && <p className="text-red-400 text-center mt-4">{error}</p>}
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-900">
      <form onSubmit={handleSubmit} className="bg-gray-800 p-8 rounded-lg shadow-xl max-w-md w-full">
        <h1 className="text-2xl font-bold text-center mb-6">Sign Challenge</h1>
        <div className="mb-4">
          <label className="block text-gray-400 text-sm mb-2">Challenge</label>
          <div className="bg-gray-700 p-3 rounded text-sm break-all font-mono">
            {challenge}
          </div>
        </div>
        <div className="mb-6">
          <label className="block text-gray-400 text-sm mb-2">Signature</label>
          <textarea
            value={signature}
            onChange={(e) => setSignature(e.target.value)}
            className="w-full bg-gray-700 text-white p-3 rounded border border-gray-600 focus:border-blue-500 focus:outline-none font-mono text-sm"
            rows={4}
            placeholder="Paste your signature here..."
            required
          />
        </div>
        <button
          type="submit"
          disabled={loading || !signature}
          className="w-full bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 text-white py-2 px-4 rounded transition-colors"
        >
          {loading ? 'Verifying...' : 'Authenticate'}
        </button>
        {error && <p className="text-red-400 text-center mt-4">{error}</p>}
      </form>
    </div>
  )
}

import { useState, KeyboardEvent } from 'react'

interface PostComposerProps {
  onPost: (content: string) => void
}

export function PostComposer({ onPost }: PostComposerProps) {
  const [content, setContent] = useState('')

  const handleSubmit = () => {
    const trimmed = content.trim()
    if (trimmed) {
      onPost(trimmed)
      setContent('')
    }
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSubmit()
    }
  }

  return (
    <div className="flex gap-2">
      <textarea
        value={content}
        onChange={(e) => setContent(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Type a message... (Enter to send, Shift+Enter for new line)"
        className="flex-1 bg-gray-800 text-white p-3 rounded-lg border border-gray-600 focus:border-blue-500 focus:outline-none resize-none"
        rows={2}
      />
      <button
        onClick={handleSubmit}
        disabled={!content.trim()}
        className="bg-blue-600 hover:bg-blue-700 disabled:bg-gray-600 disabled:cursor-not-allowed text-white px-6 rounded-lg transition-colors"
      >
        Send
      </button>
    </div>
  )
}

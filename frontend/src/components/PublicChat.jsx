import React, { useState, useEffect } from 'react'
import { MessageSquare, Send, Bot as BotIcon } from 'lucide-react'
import ThemeToggle from './ThemeToggle'
import './BotChat.css'

const API_BASE = '/api/v1'

// Public-facing chat page. Rendered when the URL matches /chat/:botId.
// Does NOT require authentication — fetches the bot's public info and uses
// the public chat endpoint, which runs with the owner's saved generation
// settings server-side.
function PublicChat({ botId }) {
  const [bot, setBot] = useState(null)
  const [loadError, setLoadError] = useState('')
  const [isLoadingBot, setIsLoadingBot] = useState(true)

  const [messages, setMessages] = useState([])
  const [inputMessage, setInputMessage] = useState('')
  const [isLoading, setIsLoading] = useState(false)

  useEffect(() => {
    let cancelled = false
    setIsLoadingBot(true)
    setLoadError('')
    fetch(`${API_BASE}/bots/${botId}`)
      .then(async (r) => {
        if (!r.ok) {
          const data = await r.json().catch(() => ({}))
          throw new Error(data.error || `HTTP ${r.status}`)
        }
        return r.json()
      })
      .then((data) => {
        if (cancelled) return
        setBot(data)
        setIsLoadingBot(false)
      })
      .catch((err) => {
        if (cancelled) return
        setLoadError(err.message || 'Failed to load bot')
        setIsLoadingBot(false)
      })
    return () => {
      cancelled = true
    }
  }, [botId])

  const handleSendMessage = async (e) => {
    e.preventDefault()
    if (!inputMessage.trim() || isLoading || !bot) return

    const userMessage = { role: 'user', content: inputMessage }
    setMessages((prev) => [...prev, userMessage])
    const query = inputMessage
    setInputMessage('')
    setIsLoading(true)

    const assistantMessage = { role: 'assistant', content: '', streaming: true }
    setMessages((prev) => [...prev, assistantMessage])

    try {
      // Public endpoint ignores generation params from the client — the backend
      // pulls them from the bot's saved config. We only send the message.
      const response = await fetch(`${API_BASE}/chat/public/${bot.id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: query, limit: 60 })
      })

      if (!response.ok) throw new Error('Chat failed')

      const reader = response.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const data = line.slice(6)
          if (data === '[DONE]') break

          try {
            const parsed = JSON.parse(data)
            if (parsed.type === 'token' && parsed.token) {
              setMessages((prev) => {
                const next = [...prev]
                const last = next[next.length - 1]
                if (last.role === 'assistant') last.content += parsed.token
                return next
              })
            }
            if (parsed.type === 'error') {
              setMessages((prev) => {
                const next = [...prev]
                next[next.length - 1] = {
                  role: 'assistant',
                  content: `Error: ${parsed.error}`,
                  streaming: false
                }
                return next
              })
              break
            }
            if (parsed.type === 'done') break
          } catch {
            // skip non-JSON frames
          }
        }
      }

      setMessages((prev) => {
        const next = [...prev]
        const last = next[next.length - 1]
        if (last.role === 'assistant') last.streaming = false
        return next
      })
    } catch (err) {
      console.error('Public chat error:', err)
      setMessages((prev) => {
        const next = [...prev]
        next[next.length - 1] = {
          role: 'assistant',
          content: 'Error: Failed to get response',
          streaming: false
        }
        return next
      })
    } finally {
      setIsLoading(false)
    }
  }

  if (isLoadingBot) {
    return (
      <div className="bot-chat-container">
        <div className="empty-chat">
          <BotIcon size={56} strokeWidth={1.5} />
          <h2>Loading bot...</h2>
        </div>
      </div>
    )
  }

  if (loadError || !bot) {
    return (
      <div className="bot-chat-container">
        <div className="empty-chat">
          <BotIcon size={56} strokeWidth={1.5} />
          <h2>Bot unavailable</h2>
          <p>{loadError || 'This bot does not exist or is not active.'}</p>
        </div>
      </div>
    )
  }

  return (
    <div className="bot-chat-container">
      <header className="bot-chat-header">
        <div className="bot-info">
          <h1>{bot.name}</h1>
          <p>{bot.description || 'Public chat'}</p>
        </div>
        <ThemeToggle />
      </header>

      <div className="chat-messages">
        {messages.length === 0 ? (
          <div className="empty-chat">
            <MessageSquare size={64} />
            <h2>Start a conversation</h2>
            <p>Ask questions about this bot's knowledge base</p>
          </div>
        ) : (
          messages.map((msg, idx) => (
            <div key={idx} className={`message ${msg.role}`}>
              <div className="message-content">
                {msg.content}
                {msg.streaming && <span className="cursor">▊</span>}
              </div>
            </div>
          ))
        )}
      </div>

      <form onSubmit={handleSendMessage} className="chat-input-form">
        <input
          type="text"
          value={inputMessage}
          onChange={(e) => setInputMessage(e.target.value)}
          placeholder="Ask a question..."
          disabled={isLoading}
          className="chat-input"
        />
        <button
          type="submit"
          disabled={isLoading || !inputMessage.trim()}
          className="send-btn"
        >
          <Send size={20} />
        </button>
      </form>
    </div>
  )
}

export default PublicChat

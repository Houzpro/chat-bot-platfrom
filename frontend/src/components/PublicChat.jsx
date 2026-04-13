import React, { useState, useEffect, useRef } from 'react'
import { MessageSquare, Send, Bot as BotIcon, StopCircle, ThumbsUp, ThumbsDown } from 'lucide-react'
import { publicFeedbackAPI } from '../api/client'
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
  const abortControllerRef = useRef(null)

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

  const handleFeedback = async (msgIdx, rating) => {
    const msg = messages[msgIdx]
    if (!msg.id || msg.feedback) return // immutable — skip if already rated

    try {
      await publicFeedbackAPI.submit(msg.id, rating)
      setMessages(prev => {
        const next = [...prev]
        next[msgIdx] = { ...next[msgIdx], feedback: rating }
        return next
      })
    } catch (err) {
      console.error('Failed to submit feedback:', err)
    }
  }

  const handleStopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
  }

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

    abortControllerRef.current = new AbortController()

    try {
      // Send recent history so the model has conversational context.
      // Trim on the client to avoid bloated requests on long sessions.
      const contextWindow = bot.context_window || 10
      const completedMessages = messages
        .filter(m => !m.streaming)
        .map(m => ({ role: m.role, content: m.content }))
      const history = completedMessages.slice(-contextWindow)
      const response = await fetch(`${API_BASE}/chat/public/${bot.id}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        signal: abortControllerRef.current.signal,
        body: JSON.stringify({
          message: query,
          limit: 60,
          history,
          context_window: contextWindow,
        })
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
            if (parsed.type === 'done') {
              if (parsed.message_id) {
                setMessages((prev) => {
                  const next = [...prev]
                  const last = next[next.length - 1]
                  if (last.role === 'assistant') last.id = parsed.message_id
                  return next
                })
              }
              break
            }
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
      if (err.name === 'AbortError') {
        setMessages((prev) => {
          const next = [...prev]
          const last = next[next.length - 1]
          if (last.role === 'assistant') {
            last.streaming = false
            last.cancelled = true
          }
          return next
        })
      } else {
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
      }
    } finally {
      abortControllerRef.current = null
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
                {msg.cancelled && <span className="cancelled-label"> (stopped)</span>}
              </div>
              {msg.role === 'assistant' && !msg.streaming && msg.id && (
                <div className={`message-feedback${msg.feedback ? ' rated' : ''}`}>
                  <button
                    className={`feedback-btn${msg.feedback === 1 ? ' active positive' : ''}`}
                    onClick={() => handleFeedback(idx, 1)}
                    title="Полезный ответ"
                  >
                    <ThumbsUp size={14} />
                  </button>
                  <button
                    className={`feedback-btn${msg.feedback === -1 ? ' active negative' : ''}`}
                    onClick={() => handleFeedback(idx, -1)}
                    title="Неполезный ответ"
                  >
                    <ThumbsDown size={14} />
                  </button>
                </div>
              )}
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
        {isLoading ? (
          <button type="button" onClick={handleStopGeneration} className="send-btn stop-btn">
            <StopCircle size={20} />
          </button>
        ) : (
          <button
            type="submit"
            disabled={!inputMessage.trim()}
            className="send-btn"
          >
            <Send size={20} />
          </button>
        )}
      </form>
    </div>
  )
}

export default PublicChat

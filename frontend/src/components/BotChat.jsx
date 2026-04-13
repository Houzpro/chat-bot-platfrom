import React, { useState, useRef, useEffect } from 'react'
import { ArrowLeft, MessageSquare, Send, StopCircle, Plus, Trash2 } from 'lucide-react'
import { conversationsAPI } from '../api/client'
import ThemeToggle from './ThemeToggle'
import './BotChat.css'

const API_BASE = '/api/v1'

function BotChat({ bot, token, onBack }) {
  const [messages, setMessages] = useState([])
  const [inputMessage, setInputMessage] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [conversations, setConversations] = useState([])
  const [activeConversationId, setActiveConversationId] = useState(null)
  const abortControllerRef = useRef(null)
  const messagesEndRef = useRef(null)

  // Auto-scroll to bottom
  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  // Load conversations on mount
  useEffect(() => {
    loadConversations()
  }, [bot.id])

  const loadConversations = async () => {
    try {
      const convs = await conversationsAPI.getByBot(bot.id)
      setConversations(convs)
    } catch (err) {
      console.error('Failed to load conversations:', err)
    }
  }

  const createNewConversation = async () => {
    try {
      const conv = await conversationsAPI.create(bot.id)
      setConversations(prev => [conv, ...prev])
      setActiveConversationId(conv.id)
      setMessages([])
    } catch (err) {
      console.error('Failed to create conversation:', err)
    }
  }

  const selectConversation = async (convId) => {
    if (convId === activeConversationId) return
    setActiveConversationId(convId)
    try {
      const data = await conversationsAPI.get(convId)
      setMessages(
        (data.messages || []).map(m => ({
          role: m.role,
          content: m.content,
          streaming: false,
        }))
      )
    } catch (err) {
      console.error('Failed to load conversation:', err)
      setMessages([])
    }
  }

  const deleteConversation = async (convId, e) => {
    e.stopPropagation()
    try {
      await conversationsAPI.delete(convId)
      setConversations(prev => prev.filter(c => c.id !== convId))
      if (activeConversationId === convId) {
        setActiveConversationId(null)
        setMessages([])
      }
    } catch (err) {
      console.error('Failed to delete conversation:', err)
    }
  }

  const handleStopGeneration = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
  }

  const handleSendMessage = async (e) => {
    e.preventDefault()
    if (!inputMessage.trim() || isLoading) return

    // Auto-create conversation if none active
    let convId = activeConversationId
    if (!convId) {
      try {
        const conv = await conversationsAPI.create(bot.id)
        convId = conv.id
        setActiveConversationId(conv.id)
        setConversations(prev => [conv, ...prev])
      } catch (err) {
        console.error('Failed to create conversation:', err)
        return
      }
    }

    const userMessage = { role: 'user', content: inputMessage }
    setMessages(prev => [...prev, userMessage])
    const query = inputMessage
    setInputMessage('')
    setIsLoading(true)

    const assistantMessage = { role: 'assistant', content: '', streaming: true }
    setMessages(prev => [...prev, assistantMessage])

    abortControllerRef.current = new AbortController()

    try {
      const response = await fetch(`${API_BASE}/chat/rag`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        signal: abortControllerRef.current.signal,
        body: JSON.stringify({
          client_id: bot.id,
          query: query,
          conversation_id: convId,
          limit: 60,
          temperature: bot.temperature,
          top_p: bot.top_p,
          top_k: bot.top_k,
          max_new_tokens: bot.max_new_tokens,
          do_sample: bot.do_sample,
          system_prompt: bot.system_prompt,
          context_window: bot.context_window || 0
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
          if (line.startsWith('data: ')) {
            const data = line.slice(6)
            if (data === '[DONE]') break

            try {
              const parsed = JSON.parse(data)

              if (parsed.type === 'meta' && parsed.conversation_id) {
                continue
              }

              if (parsed.type === 'token' && parsed.token) {
                setMessages(prev => {
                  const newMessages = [...prev]
                  const lastMsg = newMessages[newMessages.length - 1]
                  if (lastMsg.role === 'assistant') {
                    lastMsg.content += parsed.token
                  }
                  return newMessages
                })
              }
              if (parsed.type === 'error') {
                setMessages(prev => {
                  const newMessages = [...prev]
                  newMessages[newMessages.length - 1] = {
                    role: 'assistant',
                    content: `Error: ${parsed.error}`,
                    streaming: false
                  }
                  return newMessages
                })
                break
              }
              if (parsed.type === 'done') break
            } catch (e) {
              // Skip non-JSON lines
            }
          }
        }
      }

      setMessages(prev => {
        const newMessages = [...prev]
        const lastMsg = newMessages[newMessages.length - 1]
        if (lastMsg.role === 'assistant') {
          lastMsg.streaming = false
        }
        return newMessages
      })

      // Refresh conversations to update titles
      loadConversations()

    } catch (err) {
      if (err.name === 'AbortError') {
        setMessages(prev => {
          const newMessages = [...prev]
          const lastMsg = newMessages[newMessages.length - 1]
          if (lastMsg.role === 'assistant') {
            lastMsg.streaming = false
            lastMsg.cancelled = true
          }
          return newMessages
        })
      } else {
        console.error('Chat error:', err)
        setMessages(prev => {
          const newMessages = [...prev]
          newMessages[newMessages.length - 1] = {
            role: 'assistant',
            content: 'Error: Failed to get response',
            streaming: false
          }
          return newMessages
        })
      }
    } finally {
      abortControllerRef.current = null
      setIsLoading(false)
    }
  }

  return (
    <div className="bot-chat-container">
      <header className="bot-chat-header">
        <button onClick={onBack} className="back-btn">
          <ArrowLeft size={18} />
          <span>Back</span>
        </button>
        <div className="bot-info">
          <h1>{bot.name}</h1>
          <p>{bot.description || 'No description'}</p>
        </div>
        <ThemeToggle />
      </header>

      <div className="chat-body">
        {/* Sidebar */}
        <div className="chat-sidebar">
          <div className="sidebar-header">
            <h3>Conversations</h3>
            <button onClick={createNewConversation} className="new-conv-btn" title="New conversation">
              <Plus size={18} />
            </button>
          </div>
          <div className="sidebar-list">
            {conversations.map(conv => (
              <div
                key={conv.id}
                className={`sidebar-item ${conv.id === activeConversationId ? 'active' : ''}`}
                onClick={() => selectConversation(conv.id)}
              >
                <span className="conv-title">{conv.title || 'New conversation'}</span>
                <button
                  className="conv-delete"
                  onClick={(e) => deleteConversation(conv.id, e)}
                  title="Delete"
                >
                  <Trash2 size={14} />
                </button>
              </div>
            ))}
            {conversations.length === 0 && (
              <div className="sidebar-empty">No conversations yet</div>
            )}
          </div>
        </div>

        {/* Chat area */}
        <div className="chat-main">
          <div className="chat-messages">
            {messages.length === 0 ? (
              <div className="empty-chat">
                <MessageSquare size={64} />
                <h2>Start a conversation</h2>
                <p>Ask questions about your uploaded documents</p>
              </div>
            ) : (
              messages.map((msg, idx) => (
                <div key={idx} className={`message ${msg.role}`}>
                  <div className="message-content">
                    {msg.content}
                    {msg.streaming && <span className="cursor">▊</span>}
                    {msg.cancelled && <span className="cancelled-label"> (stopped)</span>}
                  </div>
                </div>
              ))
            )}
            <div ref={messagesEndRef} />
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
              <button type="submit" disabled={!inputMessage.trim()} className="send-btn">
                <Send size={20} />
              </button>
            )}
          </form>
        </div>
      </div>
    </div>
  )
}

export default BotChat

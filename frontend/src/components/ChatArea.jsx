import React, { useRef, useEffect, useState } from 'react'
import { Send, Loader, StopCircle, ThumbsUp, ThumbsDown } from 'lucide-react'
import { feedbackAPI } from '../api/client'
import './ChatArea.css'

function ChatArea({ messages, onSendMessage, onStopGeneration, isLoading, onUpdateMessage }) {
  const [input, setInput] = useState('')
  const messagesEndRef = useRef(null)

  useEffect(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  const handleFeedback = async (msgIdx, rating) => {
    const msg = messages[msgIdx]
    if (!msg.id || msg.feedback) return // immutable — skip if already rated

    try {
      await feedbackAPI.submit(msg.id, rating)
      onUpdateMessage?.(msgIdx, { feedback: rating })
    } catch (err) {
      console.error('Failed to submit feedback:', err)
    }
  }

  const handleSubmit = (e) => {
    e.preventDefault()
    if (input.trim() && !isLoading) {
      onSendMessage(input)
      setInput('')
    }
  }

  return (
    <div className="chat-area">
      <div className="messages">
        {messages.length === 0 ? (
          <div className="empty-state">
            <div className="empty-icon">💬</div>
            <h3>Начните разговор</h3>
            <p>Загрузите документы и задайте вопросы</p>
          </div>
        ) : (
          messages.map((msg, idx) => (
            <div key={idx} className={`message ${msg.role}`}>
              <div className="message-avatar">
                {msg.role === 'user' ? '👤' : '🤖'}
              </div>
              <div className="message-content">
                <div className="message-text">
                  {msg.content || (msg.streaming && <Loader className="spinner" size={16} />)}
                </div>
                {msg.documents && msg.documents.length > 0 && (
                  <div className="message-documents">
                    <div className="documents-header">
                      📚 Использованы документы ({msg.documents.length}):
                    </div>
                    {msg.documents.map((doc, i) => (
                      <div key={i} className="document-item">
                        <div className="doc-score">
                          {(doc.score * 100).toFixed(1)}%
                        </div>
                        <div className="doc-text">{doc.text}</div>
                      </div>
                    ))}
                  </div>
                )}
                {msg.error && (
                  <div className="message-error">❌ Ошибка при отправке сообщения</div>
                )}
                {msg.cancelled && (
                  <div className="message-cancelled">⏸️ Генерация остановлена</div>
                )}
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
            </div>
          ))
        )}
        <div ref={messagesEndRef} />
      </div>

      <form className="chat-input" onSubmit={handleSubmit}>
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          placeholder="Напишите ваш вопрос..."
          disabled={isLoading}
        />
        {isLoading ? (
          <button type="button" onClick={onStopGeneration} className="stop-btn">
            <StopCircle size={20} />
          </button>
        ) : (
          <button type="submit" disabled={!input.trim()}>
            <Send size={20} />
          </button>
        )}
      </form>
    </div>
  )
}

export default ChatArea

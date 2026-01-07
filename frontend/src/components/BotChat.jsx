import React, { useState } from 'react'
import { ArrowLeft, Upload, MessageSquare, Send } from 'lucide-react'
import './BotChat.css'

const API_BASE = 'http://localhost:8080/api/v1'

function BotChat({ bot, token, onBack }) {
  const [messages, setMessages] = useState([])
  const [inputMessage, setInputMessage] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [uploadStatus, setUploadStatus] = useState('')

  const handleFileUpload = async (e) => {
    const file = e.target.files[0]
    if (!file) return

    setUploadStatus('Uploading...')
    const formData = new FormData()
    formData.append('file', file)

    try {
      const response = await fetch(`${API_BASE}/bots/${bot.id}/documents/upload`, {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`
        },
        body: formData
      })

      if (response.ok) {
        const data = await response.json()
        setUploadStatus(`✓ Uploaded: ${data.file_name}`)
        setTimeout(() => setUploadStatus(''), 3000)
      } else {
        const error = await response.json()
        setUploadStatus(`✗ Error: ${error.error}`)
      }
    } catch (err) {
      setUploadStatus('✗ Upload failed')
      console.error('Upload error:', err)
    }
  }

  const handleSendMessage = async (e) => {
    e.preventDefault()
    if (!inputMessage.trim() || isLoading) return

    const userMessage = { role: 'user', content: inputMessage }
    setMessages(prev => [...prev, userMessage])
    setInputMessage('')
    setIsLoading(true)

    const assistantMessage = { role: 'assistant', content: '', streaming: true }
    setMessages(prev => [...prev, assistantMessage])

    try {
      const response = await fetch(`${API_BASE}/chat/public/${bot.id}`, {
        method: 'POST',
        headers: { 
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({
          message: inputMessage,
          limit: 3,
          temperature: bot.temperature,
          top_p: bot.top_p,
          top_k: bot.top_k,
          max_new_tokens: bot.max_new_tokens,
          do_sample: bot.do_sample,
          system_prompt: bot.system_prompt
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
              
              // Handle token streaming
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
              
              // Handle errors
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
              
              // Handle completion
              if (parsed.type === 'done') {
                break
              }
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

    } catch (err) {
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
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <div className="bot-chat-container">
      <header className="bot-chat-header">
        <button onClick={onBack} className="back-btn">
          <ArrowLeft size={20} />
          Back to Dashboard
        </button>
        <div className="bot-info">
          <h1>{bot.name}</h1>
          <p>{bot.description || 'No description'}</p>
        </div>
        <div className="upload-section">
          <label className="upload-btn">
            <Upload size={20} />
            Upload Document
            <input type="file" onChange={handleFileUpload} accept=".pdf,.txt,.docx,.csv,.json,.md,.html" hidden />
          </label>
          {uploadStatus && <span className="upload-status">{uploadStatus}</span>}
        </div>
      </header>

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
        <button type="submit" disabled={isLoading || !inputMessage.trim()} className="send-btn">
          <Send size={20} />
        </button>
      </form>
    </div>
  )
}

export default BotChat

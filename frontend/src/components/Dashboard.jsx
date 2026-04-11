import React, { useState, useEffect, useRef } from 'react'
import { Plus, Bot, LogOut, Trash2, Edit, ExternalLink, Upload } from 'lucide-react'
import BotChat from './BotChat'
import BotForm from './BotForm'
import './Dashboard.css'

const API_BASE = '/api/v1'

const DEFAULT_MAX_FILE_SIZE = 20 * 1024 * 1024
const DEFAULT_ALLOWED_EXTENSIONS = ['.pdf', '.txt', '.docx', '.doc', '.csv', '.xlsx', '.json', '.md', '.html']

const formatBytes = (bytes) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit++
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

function Dashboard({ token, user, onLogout }) {
  const [bots, setBots] = useState([])
  const [selectedBot, setSelectedBot] = useState(null)
  const [showBotForm, setShowBotForm] = useState(false)
  const [editingBot, setEditingBot] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const [uploadStatus, setUploadStatus] = useState({}) // { [botId]: { type: 'info'|'success'|'error', message: string } }
  const [maxFileSize, setMaxFileSize] = useState(DEFAULT_MAX_FILE_SIZE)
  const [allowedExtensions, setAllowedExtensions] = useState(DEFAULT_ALLOWED_EXTENSIONS)
  const fileInputsRef = useRef({}) // { [botId]: HTMLInputElement }

  useEffect(() => {
    loadBots()
    // Fetch upload limits so .env remains the single source of truth
    fetch(`${API_BASE}/config/defaults`)
      .then(r => r.ok ? r.json() : null)
      .then(defaults => {
        if (!defaults) return
        if (Number.isFinite(defaults.max_file_size) && defaults.max_file_size > 0) {
          setMaxFileSize(defaults.max_file_size)
        }
        if (Array.isArray(defaults.allowed_extensions) && defaults.allowed_extensions.length > 0) {
          setAllowedExtensions(defaults.allowed_extensions)
        }
      })
      .catch(() => {})
  }, [])

  const setBotStatus = (botId, type, message, autoClearMs = 0) => {
    setUploadStatus(prev => ({ ...prev, [botId]: { type, message } }))
    if (autoClearMs > 0) {
      setTimeout(() => {
        setUploadStatus(prev => {
          const next = { ...prev }
          delete next[botId]
          return next
        })
      }, autoClearMs)
    }
  }

  const handleUploadClick = (botId) => {
    const input = fileInputsRef.current[botId]
    if (input) input.click()
  }

  const handleUploadFile = async (botId, e) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return

    const name = file.name.toLowerCase()
    const extOk = allowedExtensions.some(ext => name.endsWith(ext))
    if (!extOk) {
      setBotStatus(botId, 'error', `Unsupported type (allowed: ${allowedExtensions.join(', ')})`, 5000)
      return
    }
    if (file.size > maxFileSize) {
      setBotStatus(botId, 'error', `File exceeds ${formatBytes(maxFileSize)}`, 5000)
      return
    }

    setBotStatus(botId, 'info', `Uploading ${file.name}...`)
    const formData = new FormData()
    formData.append('file', file)

    try {
      const response = await fetch(`${API_BASE}/bots/${botId}/documents/upload`, {
        method: 'POST',
        headers: { 'Authorization': `Bearer ${token}` },
        body: formData,
      })
      if (response.ok) {
        const data = await response.json()
        setBotStatus(botId, 'success', `Uploaded: ${data.file_name || file.name}`, 4000)
      } else {
        const err = await response.json().catch(() => ({}))
        setBotStatus(botId, 'error', err.error || `HTTP ${response.status}`, 5000)
      }
    } catch (err) {
      setBotStatus(botId, 'error', 'Upload failed', 5000)
      console.error('Upload error:', err)
    }
  }

  const loadBots = async () => {
    setIsLoading(true)
    setError('')
    try {
      const response = await fetch(`${API_BASE}/bots`, {
        headers: {
          'Authorization': `Bearer ${token}`
        }
      })

      if (response.ok) {
        const data = await response.json()
        setBots(data.bots || [])
      } else if (response.status === 401) {
        onLogout()
      } else {
        setError('Failed to load bots')
      }
    } catch (err) {
      setError('Network error')
      console.error('Load bots error:', err)
    } finally {
      setIsLoading(false)
    }
  }

  const handleCreateBot = () => {
    setEditingBot(null)
    setShowBotForm(true)
  }

  const handleEditBot = (bot) => {
    setEditingBot(bot)
    setShowBotForm(true)
  }

  const handleDeleteBot = async (botId) => {
    if (!confirm('Are you sure you want to delete this bot?')) return

    try {
      const response = await fetch(`${API_BASE}/bots/${botId}`, {
        method: 'DELETE',
        headers: {
          'Authorization': `Bearer ${token}`
        }
      })

      if (response.ok) {
        setBots(bots.filter(b => b.id !== botId))
        if (selectedBot?.id === botId) {
          setSelectedBot(null)
        }
      } else {
        alert('Failed to delete bot')
      }
    } catch (err) {
      alert('Network error')
      console.error('Delete bot error:', err)
    }
  }

  const handleBotSaved = () => {
    setShowBotForm(false)
    setEditingBot(null)
    loadBots()
  }

  const handleSelectBot = (bot) => {
    setSelectedBot(bot)
  }

  const copyPublicUrl = (botId) => {
    const url = `${window.location.origin}/chat/${botId}`
    navigator.clipboard.writeText(url)
    alert('Public chat URL copied to clipboard!')
  }

  if (selectedBot) {
    return (
      <BotChat 
        bot={selectedBot} 
        token={token}
        onBack={() => setSelectedBot(null)} 
      />
    )
  }

  if (showBotForm) {
    return (
      <BotForm
        token={token}
        bot={editingBot}
        onSave={handleBotSaved}
        onCancel={() => {
          setShowBotForm(false)
          setEditingBot(null)
        }}
      />
    )
  }

  return (
    <div className="dashboard">
      <header className="dashboard-header">
        <div className="header-content">
          <div className="header-left">
            <Bot size={32} />
            <div>
              <h1>My Bots</h1>
              <p>Welcome, {user.name}</p>
            </div>
          </div>
          <div className="header-right">
            <button onClick={handleCreateBot} className="create-bot-btn">
              <Plus size={20} />
              Create Bot
            </button>
            <button onClick={onLogout} className="logout-btn">
              <LogOut size={20} />
              Logout
            </button>
          </div>
        </div>
      </header>

      <main className="dashboard-main">
        {isLoading ? (
          <div className="loading">Loading bots...</div>
        ) : error ? (
          <div className="error-state">{error}</div>
        ) : bots.length === 0 ? (
          <div className="empty-state">
            <Bot size={64} />
            <h2>No bots yet</h2>
            <p>Create your first bot to get started</p>
            <button onClick={handleCreateBot} className="create-bot-btn">
              <Plus size={20} />
              Create Bot
            </button>
          </div>
        ) : (
          <div className="bots-grid">
            {bots.map(bot => (
              <div key={bot.id} className="bot-card">
                <div className="bot-card-header">
                  <div className="bot-icon">
                    <Bot size={24} />
                  </div>
                  <div className="bot-status">
                    <span className={`status-badge ${bot.is_active ? 'active' : 'inactive'}`}>
                      {bot.is_active ? 'Active' : 'Inactive'}
                    </span>
                  </div>
                </div>
                
                <h3>{bot.name}</h3>
                <p className="bot-description">{bot.description || 'No description'}</p>
                
                <div className="bot-stats">
                  <div className="stat">
                    <span className="stat-label">Temperature</span>
                    <span className="stat-value">{bot.temperature}</span>
                  </div>
                  <div className="stat">
                    <span className="stat-label">Max Tokens</span>
                    <span className="stat-value">{bot.max_new_tokens}</span>
                  </div>
                </div>

                <div className="bot-actions">
                  <button
                    onClick={() => handleSelectBot(bot)}
                    className="action-btn primary"
                  >
                    Open Chat
                  </button>
                  <button
                    onClick={() => handleUploadClick(bot.id)}
                    className="action-btn"
                    title={`Upload document (max ${formatBytes(maxFileSize)})`}
                  >
                    <Upload size={16} />
                  </button>
                  <input
                    ref={el => { fileInputsRef.current[bot.id] = el }}
                    type="file"
                    accept={allowedExtensions.join(',')}
                    onChange={(e) => handleUploadFile(bot.id, e)}
                    style={{ display: 'none' }}
                  />
                  <button
                    onClick={() => copyPublicUrl(bot.id)}
                    className="action-btn"
                    title="Copy public URL"
                  >
                    <ExternalLink size={16} />
                  </button>
                  <button
                    onClick={() => handleEditBot(bot)}
                    className="action-btn"
                    title="Edit bot"
                  >
                    <Edit size={16} />
                  </button>
                  <button
                    onClick={() => handleDeleteBot(bot.id)}
                    className="action-btn danger"
                    title="Delete bot"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
                {uploadStatus[bot.id] && (
                  <div className={`bot-upload-status ${uploadStatus[bot.id].type}`}>
                    {uploadStatus[bot.id].message}
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </main>
    </div>
  )
}

export default Dashboard

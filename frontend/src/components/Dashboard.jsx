import React, { useState, useEffect, useRef } from 'react'
import { Plus, Bot, LogOut, Trash2, Edit, ExternalLink, Upload, BarChart2, Search, X, ShieldCheck } from 'lucide-react'
import BotChat from './BotChat'
import BotForm from './BotForm'
import Analytics from './Analytics'
import Pagination from './Pagination'
import ThemeToggle from './ThemeToggle'
import { botsAPI } from '../api/client'
import { usePagination } from '../hooks/usePagination'
import { useDebouncedValue } from '../hooks/useDebouncedValue'
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

function Dashboard({ token, user, onLogout, activeBotId = null, analyticsBotId = null, navigate }) {
  const [bots, setBots] = useState([])
  const [showBotForm, setShowBotForm] = useState(false)
  const [editingBot, setEditingBot] = useState(null)
  const [isLoading, setIsLoading] = useState(true)
  const [error, setError] = useState('')
  const botsPage = usePagination({ page: 1, limit: 12 })
  const [reloadNonce, setReloadNonce] = useState(0)
  const [searchInput, setSearchInput] = useState('')
  // Debounce input so typing doesn't spam the backend. 300ms feels snappy
  // but lets the user finish typing short tokens before we query.
  const searchQuery = useDebouncedValue(searchInput.trim(), 300)
  const [uploadStatus, setUploadStatus] = useState({}) // { [botId]: { type: 'info'|'success'|'error', message: string } }
  const [maxFileSize, setMaxFileSize] = useState(DEFAULT_MAX_FILE_SIZE)
  const [allowedExtensions, setAllowedExtensions] = useState(DEFAULT_ALLOWED_EXTENSIONS)
  const fileInputsRef = useRef({}) // { [botId]: HTMLInputElement }

  useEffect(() => {
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

  // Reload bots whenever the active page changes. Pulling this into an effect
  // (rather than a one-shot call in mount) means page clicks transparently
  // refetch without components needing to know about loadBots.
  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setIsLoading(true)
      setError('')
      try {
        const data = await botsAPI.getMyBots({
          page: botsPage.page,
          limit: botsPage.limit,
          search: searchQuery,
        })
        if (cancelled) return
        setBots(data.items || [])
        botsPage.setMeta(data.pagination || null)
        // If items were deleted on another tab and our page is now past the end,
        // snap back to the last real page so the UI doesn't show an empty grid.
        const meta = data.pagination
        if (meta && meta.total_pages > 0 && botsPage.page > meta.total_pages) {
          botsPage.setPage(meta.total_pages)
        }
      } catch (err) {
        if (cancelled) return
        if (err.message === 'HTTP 401') {
          onLogout()
          return
        }
        setError(err.message || 'Failed to load bots')
        console.error('Load bots error:', err)
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [botsPage.page, botsPage.limit, reloadNonce, searchQuery])

  // Changing the query must snap back to page 1 — otherwise the user can be
  // stranded on e.g. page 4 of a filter that only has 1 page of results.
  // `botsPage.setPage` is stable, and using a ref-less setter would require
  // threading it; the ESLint rule's noise is the reason for the eslint-disable.
  useEffect(() => {
    botsPage.setPage(1)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchQuery])

  // Manual reload helper — bumps nonce so the effect refires even if page/limit
  // haven't changed (e.g. after creating a bot while already on page 1).
  const reloadBots = () => setReloadNonce(n => n + 1)

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
        if (activeBotId === botId) {
          navigate('/', { replace: true })
        }
        // Refetch so total/page meta stays in sync; local filter would desync counts.
        reloadBots()
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
    reloadBots()
  }

  // The open bot is derived from the URL (activeBotId from App.jsx). With
  // paginated lists the target bot might live on another page, so we keep
  // a separate `resolvedBot` slot populated by a point lookup when the
  // list doesn't contain the URL target.
  const [resolvedBot, setResolvedBot] = useState(null)

  const listMatch = (id) => bots.find(b => b.id === id) || null
  const selectedBot = activeBotId
    ? listMatch(activeBotId) || (resolvedBot && resolvedBot.id === activeBotId ? resolvedBot : null)
    : null
  const analyticsBot = analyticsBotId
    ? listMatch(analyticsBotId) || (resolvedBot && resolvedBot.id === analyticsBotId ? resolvedBot : null)
    : null

  const handleSelectBot = (bot) => {
    navigate(`/chat/${bot.id}`)
  }

  const handleOpenAnalytics = (bot) => {
    navigate(`/analytics/${bot.id}`)
  }

  const handleBackFromChat = () => {
    navigate('/')
  }

  // If the URL points at a bot that isn't on the current page, resolve it via
  // a point lookup. On 404 kick back to the dashboard so stale links don't
  // leave us on a "ghost" route.
  useEffect(() => {
    const target = activeBotId || analyticsBotId
    if (!target || isLoading) {
      if (!target) setResolvedBot(null)
      return
    }
    if (listMatch(target)) {
      setResolvedBot(null)
      return
    }
    let cancelled = false
    botsAPI.getBot(target)
      .then(bot => { if (!cancelled) setResolvedBot(bot) })
      .catch(() => { if (!cancelled) navigate('/', { replace: true }) })
    return () => { cancelled = true }
  }, [activeBotId, analyticsBotId, isLoading, bots, navigate])

  const copyPublicUrl = (botId) => {
    const url = `${window.location.origin}/public/${botId}`
    navigator.clipboard.writeText(url)
    alert('Public chat URL copied to clipboard!')
  }

  if (selectedBot) {
    return (
      <BotChat
        bot={selectedBot}
        token={token}
        onBack={handleBackFromChat}
      />
    )
  }

  if (analyticsBot) {
    return <Analytics bot={analyticsBot} onBack={handleBackFromChat} />
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
            <Bot size={24} />
            <div>
              <h1>My Bots</h1>
              <p>Welcome, {user.name}</p>
            </div>
          </div>
          <div className="header-right">
            <ThemeToggle />
            {user.role === 'admin' && (
              <button onClick={() => navigate('/admin')} className="logout-btn" title="Admin panel">
                <ShieldCheck size={18} />
                Admin
              </button>
            )}
            <button onClick={handleCreateBot} className="create-bot-btn">
              <Plus size={18} />
              Create Bot
            </button>
            <button onClick={onLogout} className="logout-btn">
              <LogOut size={18} />
              Logout
            </button>
          </div>
        </div>
      </header>

      <main className="dashboard-main">
        <div className="dashboard-toolbar">
          <div className="search-field">
            <Search size={16} className="search-icon" aria-hidden="true" />
            <input
              type="search"
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Search bots by name or description"
              aria-label="Search bots"
            />
            {searchInput && (
              <button
                type="button"
                className="search-clear"
                onClick={() => setSearchInput('')}
                aria-label="Clear search"
              >
                <X size={14} />
              </button>
            )}
          </div>
        </div>

        {isLoading ? (
          <div className="loading">Loading bots...</div>
        ) : error ? (
          <div className="error-state">{error}</div>
        ) : bots.length === 0 && searchQuery ? (
          <div className="empty-state">
            <Search size={48} strokeWidth={1.5} />
            <h2>No matches</h2>
            <p>No bots match "{searchQuery}". Try a different query.</p>
          </div>
        ) : bots.length === 0 ? (
          <div className="empty-state">
            <Bot size={56} strokeWidth={1.5} />
            <h2>No bots yet</h2>
            <p>Create your first bot to get started</p>
            <button onClick={handleCreateBot} className="create-bot-btn">
              <Plus size={18} />
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
                    {bot.role && bot.role !== 'owner' && (
                      <span
                        className="status-badge"
                        style={{
                          marginRight: 6,
                          background: 'var(--accent-muted)',
                          color: 'var(--accent)',
                          border: '1px solid var(--accent-border)',
                        }}
                      >
                        Shared · {bot.role}
                      </span>
                    )}
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
                  {/* Editors and owners may upload; viewers cannot. role is
                      'owner' for owned bots and 'editor'/'viewer' for shared. */}
                  {(bot.role === 'owner' || bot.role === 'editor' || !bot.role) && (
                    <>
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
                    </>
                  )}
                  {/* Analytics, public URL, edit, and delete are owner-only.
                      Backend enforces this with 403; we just hide the buttons. */}
                  {(bot.role === 'owner' || !bot.role) && (
                    <>
                      <button
                        onClick={() => handleOpenAnalytics(bot)}
                        className="action-btn"
                        title="Analytics"
                      >
                        <BarChart2 size={16} />
                      </button>
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
                    </>
                  )}
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
        {!isLoading && bots.length > 0 && (
          <div className="dashboard-pagination">
            <Pagination meta={botsPage.meta} onPageChange={botsPage.setPage} />
          </div>
        )}
      </main>
    </div>
  )
}

export default Dashboard

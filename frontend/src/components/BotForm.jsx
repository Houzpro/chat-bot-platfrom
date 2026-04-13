import React, { useState, useEffect } from 'react'
import { ArrowLeft, Save, Upload, X, FileText, Trash2 } from 'lucide-react'
import { botsAPI } from '../api/client'
import ThemeToggle from './ThemeToggle'
import './BotForm.css'

const API_BASE = '/api/v1'

// Ensure we never send invalid numeric values that would break backend JSON parsing
const sanitizeBotPayload = (data) => {
  const payload = { ...data }

  const floatFields = ['temperature', 'top_p']
  floatFields.forEach((field) => {
    const value = Number(payload[field])
    if (Number.isFinite(value)) {
      payload[field] = value
    } else {
      delete payload[field]
    }
  })

  const intFields = ['top_k', 'max_new_tokens', 'context_window']
  intFields.forEach((field) => {
    const value = Number(payload[field])
    if (Number.isInteger(value)) {
      payload[field] = value
    } else {
      delete payload[field]
    }
  })

  return payload
}

const DEFAULT_MAX_NEW_TOKENS_LIMIT = 8192
const DEFAULT_MAX_FILE_SIZE = 20 * 1024 * 1024 // 20 MB fallback
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

function BotForm({ token, bot, onSave, onCancel }) {
  const [formData, setFormData] = useState({
    name: bot?.name || '',
    description: bot?.description || '',
    temperature: bot?.temperature ?? 0.7,
    top_p: bot?.top_p ?? 0.9,
    top_k: bot?.top_k ?? 40,
    max_new_tokens: bot?.max_new_tokens ?? 512,
    do_sample: bot?.do_sample ?? true,
    context_window: bot?.context_window ?? 0,
    system_prompt: bot?.system_prompt || '',
    is_active: bot?.is_active ?? true
  })
  const [maxNewTokensLimit, setMaxNewTokensLimit] = useState(DEFAULT_MAX_NEW_TOKENS_LIMIT)
  const [maxFileSize, setMaxFileSize] = useState(DEFAULT_MAX_FILE_SIZE)
  const [allowedExtensions, setAllowedExtensions] = useState(DEFAULT_ALLOWED_EXTENSIONS)

  // Load server defaults (token ceiling + upload limits) from /config/defaults
  // so .env is the single source of truth. Fetched on every mount.
  useEffect(() => {
    fetch(`${API_BASE}/config/defaults`)
      .then(r => r.ok ? r.json() : null)
      .then(defaults => {
        if (!defaults) return
        if (Number.isFinite(defaults.max_new_tokens_limit) && defaults.max_new_tokens_limit > 0) {
          setMaxNewTokensLimit(defaults.max_new_tokens_limit)
        }
        if (Number.isFinite(defaults.max_file_size) && defaults.max_file_size > 0) {
          setMaxFileSize(defaults.max_file_size)
        }
        if (Array.isArray(defaults.allowed_extensions) && defaults.allowed_extensions.length > 0) {
          setAllowedExtensions(defaults.allowed_extensions)
        }
        if (!bot) {
          setFormData(prev => ({
            ...prev,
            temperature: defaults.temperature ?? prev.temperature,
            top_p: defaults.top_p ?? prev.top_p,
            top_k: defaults.top_k ?? prev.top_k,
            max_new_tokens: defaults.max_new_tokens ?? prev.max_new_tokens,
            do_sample: defaults.do_sample ?? prev.do_sample,
            context_window: defaults.context_window ?? prev.context_window,
            system_prompt: prev.system_prompt || defaults.user_prompt || '',
          }))
        }
      })
      .catch(() => {})
  }, [bot])
  const [files, setFiles] = useState([])
  const [existingDocs, setExistingDocs] = useState([])
  const [deletingDocId, setDeletingDocId] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState(null)

  // Load existing documents when editing a bot
  useEffect(() => {
    if (!bot) return
    botsAPI.getDocuments(bot.id)
      .then(data => setExistingDocs(data.documents || []))
      .catch(() => {})
  }, [bot])

  const handleFileChange = (e) => {
    const selectedFiles = Array.from(e.target.files)
    const accepted = []
    const rejected = []
    selectedFiles.forEach((file) => {
      const name = file.name.toLowerCase()
      const extOk = allowedExtensions.some(ext => name.endsWith(ext))
      if (!extOk) {
        rejected.push(`${file.name}: unsupported type`)
        return
      }
      if (file.size > maxFileSize) {
        rejected.push(`${file.name}: exceeds ${formatBytes(maxFileSize)}`)
        return
      }
      accepted.push(file)
    })
    if (rejected.length > 0) {
      setError(`Rejected files: ${rejected.join('; ')}`)
    } else {
      setError('')
    }
    if (accepted.length > 0) {
      setFiles(prev => [...prev, ...accepted])
    }
    // reset input so the same file can be re-selected after fixing
    e.target.value = ''
  }

  const removeFile = (index) => {
    setFiles(prev => prev.filter((_, i) => i !== index))
  }

  const handleDeleteDocument = async (docId, filename) => {
    if (!confirm(`Delete "${filename}"? This will also remove all associated chunks from the vector database.`)) return
    setDeletingDocId(docId)
    setError('')
    try {
      await botsAPI.deleteDocument(bot.id, docId)
      setExistingDocs(prev => prev.filter(d => d.id !== docId))
    } catch (err) {
      setError(`Failed to delete ${filename}: ${err.message}`)
    } finally {
      setDeletingDocId(null)
    }
  }

  const uploadDocuments = async (botId) => {
    if (files.length === 0) return true

    setUploadProgress('Uploading documents...')

    for (let i = 0; i < files.length; i++) {
      const file = files[i]

      setUploadProgress(`Uploading ${i + 1}/${files.length}: ${file.name}`)
      
      const formData = new FormData()
      formData.append('file', file)
      formData.append('bot_id', botId)

      try {
        const response = await fetch(`${API_BASE}/bots/${botId}/documents/upload`, {
          method: 'POST',
          headers: {
            'Authorization': `Bearer ${token}`
          },
          body: formData
        })

        if (!response.ok) {
            let message = response.statusText
            try {
              const data = await response.json()
              message = data?.error || data?.message || message
            } catch (jsonErr) {
              console.error('Failed to parse upload error response', jsonErr)
            }
            throw new Error(message)
        }
      } catch (err) {
        setError(`Failed to upload ${file.name}: ${err.message}`)
        return false
      }
    }
    
    return true
  }

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setIsLoading(true)
    setUploadProgress(null)

    try {
      const url = bot 
        ? `${API_BASE}/bots/${bot.id}` 
        : `${API_BASE}/bots`
      const payload = sanitizeBotPayload(formData)
      
      const response = await fetch(url, {
        method: bot ? 'PUT' : 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`
        },
        body: JSON.stringify(payload)
      })

      if (response.ok) {
        const savedBot = await response.json()
        const botId = savedBot.id || bot?.id
        
        // Upload documents for both new and existing bots
        if (files.length > 0) {
          const uploadSuccess = await uploadDocuments(botId)
          if (!uploadSuccess) {
            setIsLoading(false)
            return
          }
        }
        
        onSave()
      } else {
        const data = await response.json()
        setError(data.error || 'Failed to save bot')
      }
    } catch (err) {
      setError('Network error')
      console.error('Save bot error:', err)
    } finally {
      setIsLoading(false)
      setUploadProgress(null)
    }
  }

  const handleChange = (e) => {
    const { name, value, type, checked } = e.target
    setFormData({
      ...formData,
      [name]: type === 'checkbox' ? checked : 
              type === 'number' ? parseFloat(value) : 
              value
    })
  }

  return (
    <div className="bot-form-container">
      <div className="bot-form-header">
        <button onClick={onCancel} className="back-btn">
          <ArrowLeft size={18} />
          <span>Back</span>
        </button>
        <h1>{bot ? 'Edit Bot' : 'Create New Bot'}</h1>
        <ThemeToggle className="bot-form-theme-toggle" />
      </div>

      <form onSubmit={handleSubmit} className="bot-form">
        <div className="form-section">
          <h2>Basic Information</h2>
          
          <div className="form-group">
            <label htmlFor="name">Bot Name *</label>
            <input
              id="name"
              name="name"
              type="text"
              value={formData.name}
              onChange={handleChange}
              placeholder="My Assistant Bot"
              required
              minLength={3}
              maxLength={100}
              disabled={isLoading}
            />
          </div>

          <div className="form-group">
            <label htmlFor="description">Description</label>
            <textarea
              id="description"
              name="description"
              value={formData.description}
              onChange={handleChange}
              placeholder="What does this bot do?"
              maxLength={500}
              rows={3}
              disabled={isLoading}
            />
          </div>

          <div className="form-group checkbox">
            <label>
              <input
                name="is_active"
                type="checkbox"
                checked={formData.is_active}
                onChange={handleChange}
                disabled={isLoading}
              />
              <span>Active (bot is accessible)</span>
            </label>
          </div>
        </div>

        <div className="form-section">
          <h2>Generation Settings</h2>
          
          <div className="form-row">
            <div className="form-group">
              <label htmlFor="temperature">Temperature ({formData.temperature})</label>
              <input
                id="temperature"
                name="temperature"
                type="range"
                min="0"
                max="2"
                step="0.01"
                value={formData.temperature}
                onChange={handleChange}
                disabled={isLoading}
              />
              <small>Higher = more creative, Lower = more focused</small>
            </div>

            <div className="form-group">
              <label htmlFor="top_p">Top P ({formData.top_p})</label>
              <input
                id="top_p"
                name="top_p"
                type="range"
                min="0"
                max="1"
                step="0.01"
                value={formData.top_p}
                onChange={handleChange}
                disabled={isLoading}
              />
              <small>Nucleus sampling threshold</small>
            </div>
          </div>

          <div className="form-row">
            <div className="form-group">
              <label htmlFor="top_k">Top K</label>
              <input
                id="top_k"
                name="top_k"
                type="number"
                min="1"
                max="200"
                value={formData.top_k}
                onChange={handleChange}
                disabled={isLoading}
              />
              <small>Limit token choices (1-200)</small>
            </div>

            <div className="form-group">
              <label htmlFor="max_new_tokens">Max New Tokens</label>
              <input
                id="max_new_tokens"
                name="max_new_tokens"
                type="number"
                min="32"
                max={maxNewTokensLimit}
                step="32"
                value={formData.max_new_tokens}
                onChange={handleChange}
                disabled={isLoading}
              />
              <small>Maximum response length (32-{maxNewTokensLimit})</small>
            </div>
          </div>

          <div className="form-group checkbox">
            <label>
              <input
                name="do_sample"
                type="checkbox"
                checked={formData.do_sample}
                onChange={handleChange}
                disabled={isLoading}
              />
              <span>Enable sampling (recommended for creative responses)</span>
            </label>
          </div>

          <div className="form-group">
            <label htmlFor="context_window">Context Window ({formData.context_window || 0})</label>
            <input
              id="context_window"
              name="context_window"
              type="range"
              min="0"
              max="50"
              step="2"
              value={formData.context_window || 0}
              onChange={handleChange}
              disabled={isLoading}
            />
            <small>Number of recent messages sent to model for context. 0 = server default</small>
          </div>
        </div>

        <div className="form-section">
          <h2>Documents</h2>
          <p className="section-description">
            {bot ? 'Manage documents indexed for RAG' : 'Upload documents that will be indexed for RAG (Retrieval-Augmented Generation)'}
          </p>

          {/* Existing documents (edit mode) */}
          {bot && existingDocs.length > 0 && (
            <div className="files-list">
              {existingDocs.map(doc => (
                <div key={doc.id} className="file-item">
                  <FileText size={20} />
                  <span className="file-name">{doc.filename}</span>
                  <span className="file-size">{formatBytes(doc.file_size)}</span>
                  <span className="file-chunks">{doc.chunks_count} chunks</span>
                  <button
                    type="button"
                    onClick={() => handleDeleteDocument(doc.id, doc.filename)}
                    className="remove-file-btn"
                    disabled={isLoading || deletingDocId === doc.id}
                    title="Delete document and its chunks"
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
            </div>
          )}
          {bot && existingDocs.length === 0 && (
            <p className="no-documents">No documents uploaded yet</p>
          )}

          {/* Upload area */}
          <div className="upload-area">
            <input
              type="file"
              id="file-upload"
              multiple
              accept={allowedExtensions.join(',')}
              onChange={handleFileChange}
              disabled={isLoading}
              style={{ display: 'none' }}
            />
            <label htmlFor="file-upload" className="upload-label">
              <Upload size={32} />
              <span>Click to upload or drag and drop</span>
              <small>
                {allowedExtensions.map(e => e.replace('.', '').toUpperCase()).join(', ')}
                {' · max '}
                {formatBytes(maxFileSize)} per file
              </small>
            </label>
          </div>

          {files.length > 0 && (
            <div className="files-list">
              {files.map((file, index) => (
                <div key={index} className="file-item">
                  <FileText size={20} />
                  <span className="file-name">{file.name}</span>
                  <span className="file-size">
                    {formatBytes(file.size)}
                  </span>
                  <button
                    type="button"
                    onClick={() => removeFile(index)}
                    className="remove-file-btn"
                    disabled={isLoading}
                  >
                    <X size={16} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        {uploadProgress && <div className="upload-progress">{uploadProgress}</div>}
        {error && <div className="error-message">{error}</div>}

        <div className="form-section">
          <h2>System Prompt</h2>
          
          <div className="form-group">
            <label htmlFor="system_prompt">Prompt</label>
            <textarea
              id="system_prompt"
              name="system_prompt"
              value={formData.system_prompt}
              onChange={handleChange}
              placeholder="You are a helpful AI assistant..."
              maxLength={2000}
              rows={5}
              disabled={isLoading}
            />
            <small>Define the bot's personality and behavior</small>
          </div>
        </div>

        {error && <div className="error-message">{error}</div>}

        <div className="form-actions">
          <button type="button" onClick={onCancel} className="cancel-btn" disabled={isLoading}>
            Cancel
          </button>
          <button type="submit" className="save-btn" disabled={isLoading}>
            <Save size={20} />
            {isLoading ? 'Saving...' : 'Save Bot'}
          </button>
        </div>
      </form>
    </div>
  )
}

export default BotForm

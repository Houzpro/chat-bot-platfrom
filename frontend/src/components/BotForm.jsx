import React, { useState } from 'react'
import { ArrowLeft, Save, Upload, X, FileText } from 'lucide-react'
import './BotForm.css'

const API_BASE = 'http://localhost:8080/api/v1'

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

  const intFields = ['top_k', 'max_new_tokens']
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

function BotForm({ token, bot, onSave, onCancel }) {
  const [formData, setFormData] = useState({
    name: bot?.name || '',
    description: bot?.description || '',
    temperature: bot?.temperature || 0.75,
    top_p: bot?.top_p || 0.92,
    top_k: bot?.top_k || 40,
    max_new_tokens: bot?.max_new_tokens || 512,
    do_sample: bot?.do_sample ?? true,
    system_prompt: bot?.system_prompt || 'You are a helpful AI assistant.',
    is_active: bot?.is_active ?? true
  })
  const [files, setFiles] = useState([])
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(false)
  const [uploadProgress, setUploadProgress] = useState(null)

  const handleFileChange = (e) => {
    const selectedFiles = Array.from(e.target.files)
    setFiles(prev => [...prev, ...selectedFiles])
  }

  const removeFile = (index) => {
    setFiles(prev => prev.filter((_, i) => i !== index))
  }

  const uploadDocuments = async (botId) => {
    if (files.length === 0) return true

    const MAX_FILE_SIZE = 100 * 1024 * 1024 // 100MB
    const allowedExtensions = ['.pdf', '.txt', '.docx', '.doc', '.csv', '.xlsx', '.json', '.md', '.html']

    setUploadProgress('Uploading documents...')
    
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      const fileNameLower = file.name.toLowerCase()

      // Client-side validation mirrors backend limits for clearer errors
      if (file.size > MAX_FILE_SIZE) {
        setError(`File ${file.name} is too large (max 100MB)`) 
        return false
      }

      const isAllowed = allowedExtensions.some(ext => fileNameLower.endsWith(ext))
      if (!isAllowed) {
        setError(`Unsupported file type for ${file.name}. Allowed: ${allowedExtensions.join(', ')}`)
        return false
      }

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
        
        // Upload documents if creating new bot
        if (!bot && files.length > 0) {
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
          <ArrowLeft size={20} />
          Back
        </button>
        <h1>{bot ? 'Edit Bot' : 'Create New Bot'}</h1>
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
                max="4096"
                step="32"
                value={formData.max_new_tokens}
                onChange={handleChange}
                disabled={isLoading}
              />
              <small>Maximum response length</small>
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
        </div>

        {!bot && (
          <div className="form-section">
            <h2>Upload Documents</h2>
            <p className="section-description">Upload documents that will be indexed for RAG (Retrieval-Augmented Generation)</p>
            
            <div className="upload-area">
              <input
                type="file"
                id="file-upload"
                multiple
                accept=".pdf,.txt,.docx,.doc,.csv,.xlsx,.json,.md,.html"
                onChange={handleFileChange}
                disabled={isLoading}
                style={{ display: 'none' }}
              />
              <label htmlFor="file-upload" className="upload-label">
                <Upload size={32} />
                <span>Click to upload or drag and drop</span>
                <small>PDF, TXT, DOCX, CSV, XLSX, JSON, MD, HTML (max 100MB each)</small>
              </label>
            </div>

            {files.length > 0 && (
              <div className="files-list">
                {files.map((file, index) => (
                  <div key={index} className="file-item">
                    <FileText size={20} />
                    <span className="file-name">{file.name}</span>
                    <span className="file-size">
                      {(file.size / 1024).toFixed(1)} KB
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
        )}

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

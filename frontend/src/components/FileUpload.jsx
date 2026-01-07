import React, { useState, useRef } from 'react'
import { Upload, File, CheckCircle, AlertCircle, Loader } from 'lucide-react'
import './FileUpload.css'

const API_BASE = 'http://localhost:8080/api/v1'

function FileUpload({ clientId }) {
  const [isDragging, setIsDragging] = useState(false)
  const [uploading, setUploading] = useState(false)
  const [status, setStatus] = useState(null)
  const fileInputRef = useRef(null)

  const handleDragOver = (e) => {
    e.preventDefault()
    setIsDragging(true)
  }

  const handleDragLeave = () => {
    setIsDragging(false)
  }

  const handleDrop = (e) => {
    e.preventDefault()
    setIsDragging(false)
    const files = e.dataTransfer.files
    if (files.length > 0) {
      uploadFile(files[0])
    }
  }

  const handleFileSelect = (e) => {
    const files = e.target.files
    if (files.length > 0) {
      uploadFile(files[0])
    }
  }

  const uploadFile = async (file) => {
    if (!clientId) {
      setStatus({ type: 'error', message: 'Укажите Client ID' })
      return
    }

    setUploading(true)
    setStatus(null)

    const formData = new FormData()
    formData.append('file', file)
    formData.append('client_id', clientId)

    try {
      const response = await fetch(`${API_BASE}/documents/upload`, {
        method: 'POST',
        body: formData
      })

      if (!response.ok) {
        const error = await response.json()
        throw new Error(error.error || 'Upload failed')
      }

      const result = await response.json()
      const name = result.file_name || 'файл'
      setStatus({
        type: 'success',
        message: `✅ Загружено: ${name}`
      })
    } catch (error) {
      setStatus({
        type: 'error',
        message: `❌ Ошибка: ${error.message}`
      })
    } finally {
      setUploading(false)
      if (fileInputRef.current) {
        fileInputRef.current.value = ''
      }
    }
  }

  return (
    <div className="card file-upload-card">
      <h2>
        <Upload size={20} />
        Загрузка документов
      </h2>
      
      <div
        className={`upload-area ${isDragging ? 'dragging' : ''} ${uploading ? 'uploading' : ''}`}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onClick={() => !uploading && fileInputRef.current?.click()}
      >
        <input
          ref={fileInputRef}
          type="file"
          onChange={handleFileSelect}
          accept=".pdf,.txt,.docx,.xlsx,.csv,.json,.html,.md"
          style={{ display: 'none' }}
        />
        
        {uploading ? (
          <>
            <Loader className="upload-icon spinner" size={48} />
            <p>Загрузка...</p>
          </>
        ) : (
          <>
            <File className="upload-icon" size={48} />
            <p>Перетащите файл или кликните</p>
            <span className="upload-hint">PDF, TXT, DOCX, Excel, CSV, JSON, HTML, MD</span>
          </>
        )}
      </div>

      {status && (
        <div className={`upload-status ${status.type}`}>
          {status.type === 'success' ? <CheckCircle size={16} /> : <AlertCircle size={16} />}
          <span>{status.message}</span>
        </div>
      )}
    </div>
  )
}

export default FileUpload

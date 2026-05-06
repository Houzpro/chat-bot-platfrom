import React, { useEffect, useState } from 'react'
import { ArrowLeft, Cpu, Play, Square, Trash2, RefreshCw } from 'lucide-react'
import { modelsAPI } from '../api/client'
import ThemeToggle from './ThemeToggle'
import './ModelsPage.css'

// ModelsPage — registry of LLMs the user can assign to bots. Two columns:
//   • base models (shared, read-only)
//   • my fine-tuned models (private, with deploy/stop/delete)
// Phase 2 also lets you spin up a *base* model as its own container for
// testing — useful before fine-tuning lands. The platform-default
// llama-cpp container (chatbot-llama-cpp) is still off-limits to avoid
// breaking the running chat for everyone.
function ModelsPage({ onBack }) {
  const [models, setModels] = useState([])
  const [error, setError] = useState('')
  const [busyId, setBusyId] = useState(null)
  const [loading, setLoading] = useState(true)

  const reload = async () => {
    setError('')
    try {
      const data = await modelsAPI.list()
      setModels(Array.isArray(data?.items) ? data.items : [])
    } catch (err) {
      setError(err.message || 'Failed to load models')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { reload() }, [])

  const runAction = async (id, label, fn) => {
    setError('')
    setBusyId(id)
    try {
      await fn()
      await reload()
    } catch (err) {
      setError(`${label}: ${err.message || 'failed'}`)
    } finally {
      setBusyId(null)
    }
  }

  const handleDeploy = (m) => runAction(m.id, 'Deploy', () => modelsAPI.deploy(m.id))
  const handleStop = (m) => runAction(m.id, 'Stop', () => modelsAPI.stop(m.id))
  const handleDelete = (m) => {
    if (!window.confirm(`Delete model "${m.name}"? This will stop the container and unbind any bots using it.`)) return
    runAction(m.id, 'Delete', () => modelsAPI.delete(m.id))
  }

  const isPlatformDefault = (m) => m.container_name === 'chatbot-llama-cpp'

  const baseModels = models.filter(m => m.type === 'base')
  const finetunedModels = models.filter(m => m.type === 'finetuned')

  const renderRow = (m) => {
    const platformDefault = isPlatformDefault(m)
    const running = m.status === 'running'
    const busy = busyId === m.id
    return (
      <div key={m.id} className="model-row" data-status={m.status}>
        <div className="model-info">
          <Cpu size={20} />
          <div className="model-text">
            <div className="model-name">{m.name}</div>
            <div className="model-meta">
              <span className={`status-pill status-${m.status}`}>{m.status}</span>
              {m.endpoint_url && <span className="endpoint">{m.endpoint_url}</span>}
              {m.container_port > 0 && <span className="port">port {m.container_port}</span>}
            </div>
          </div>
        </div>
        <div className="model-actions">
          {!platformDefault && (
            <>
              {!running && (
                <button
                  className="btn btn-deploy"
                  onClick={() => handleDeploy(m)}
                  disabled={busy}
                  title="Start a llama.cpp container for this model"
                >
                  <Play size={16} />
                  {busy ? 'Working…' : 'Deploy'}
                </button>
              )}
              {running && (
                <button
                  className="btn btn-stop"
                  onClick={() => handleStop(m)}
                  disabled={busy}
                  title="Stop and remove the container"
                >
                  <Square size={16} />
                  Stop
                </button>
              )}
              {m.type === 'finetuned' && (
                <button
                  className="btn btn-delete"
                  onClick={() => handleDelete(m)}
                  disabled={busy}
                  title="Delete this fine-tuned model permanently"
                >
                  <Trash2 size={16} />
                </button>
              )}
            </>
          )}
          {platformDefault && (
            <span className="platform-default-note">Platform default — managed by docker-compose</span>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="models-page">
      <header className="models-header">
        <button onClick={onBack} className="back-btn">
          <ArrowLeft size={18} />
          <span>Back</span>
        </button>
        <h1>My Models</h1>
        <div className="header-actions">
          <button onClick={reload} className="refresh-btn" title="Refresh">
            <RefreshCw size={16} />
          </button>
          <ThemeToggle />
        </div>
      </header>

      {error && <div className="error-message">{error}</div>}

      {loading ? (
        <p className="empty-state">Loading…</p>
      ) : (
        <>
          <section className="model-section">
            <h2>Base models</h2>
            <p className="section-description">Shared with everyone. The platform default is managed by docker-compose; the rest can be deployed as standalone containers.</p>
            {baseModels.length === 0
              ? <p className="empty-state">No base models registered. Drop a *.gguf into ./models and restart the backend.</p>
              : baseModels.map(renderRow)}
          </section>

          <section className="model-section">
            <h2>My fine-tuned models</h2>
            <p className="section-description">Private to your account. Fine-tuning support arrives in the next phase.</p>
            {finetunedModels.length === 0
              ? <p className="empty-state">No fine-tuned models yet.</p>
              : finetunedModels.map(renderRow)}
          </section>
        </>
      )}
    </div>
  )
}

export default ModelsPage

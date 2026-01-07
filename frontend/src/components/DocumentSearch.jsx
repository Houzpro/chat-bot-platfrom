import React, { useState } from 'react'
import { Search, Loader } from 'lucide-react'
import './DocumentSearch.css'

const API_BASE = 'http://localhost:8080/api/v1'

function DocumentSearch({ clientId }) {
  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState([])

  const handleSearch = async (e) => {
    e.preventDefault()
    if (!query.trim() || !clientId || searching) return

    setSearching(true)
    setResults([])

    try {
      const response = await fetch(`${API_BASE}/search`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          client_id: clientId,
          query: query,
          limit: 5
        })
      })

      if (!response.ok) throw new Error('Search failed')

      const data = await response.json()
      setResults(data.documents || [])
    } catch (error) {
      console.error('Search error:', error)
      setResults([])
    } finally {
      setSearching(false)
    }
  }

  return (
    <div className="card search-card">
      <h2>
        <Search size={20} />
        Поиск документов
      </h2>

      <form onSubmit={handleSearch} className="search-form">
        <input
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Поисковый запрос..."
          disabled={searching}
        />
        <button type="submit" disabled={!query.trim() || !clientId || searching}>
          {searching ? <Loader className="spinner" size={18} /> : <Search size={18} />}
        </button>
      </form>

      {results.length > 0 && (
        <div className="search-results">
          <div className="results-header">Найдено: {results.length}</div>
          {results.map((doc, idx) => (
            <div key={idx} className="result-item">
              <div className="result-score">{(doc.score * 100).toFixed(1)}%</div>
              <div className="result-text">{doc.text}</div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

export default DocumentSearch

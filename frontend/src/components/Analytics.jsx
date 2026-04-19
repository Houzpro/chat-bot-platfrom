import React, { useEffect, useState } from 'react'
import { ArrowLeft, MessageSquare, MessagesSquare, ThumbsUp, ThumbsDown, Activity } from 'lucide-react'
import { analyticsAPI } from '../api/client'
import './Analytics.css'

function Analytics({ bot, onBack }) {
  const [data, setData] = useState(null)
  const [error, setError] = useState('')
  const [isLoading, setIsLoading] = useState(true)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      setIsLoading(true)
      setError('')
      try {
        const res = await analyticsAPI.getBotAnalytics(bot.id)
        if (!cancelled) setData(res)
      } catch (err) {
        if (!cancelled) setError(err.message || 'Failed to load analytics')
      } finally {
        if (!cancelled) setIsLoading(false)
      }
    }
    load()
    return () => { cancelled = true }
  }, [bot.id])

  const fb = data?.feedback
  const positivePct = fb && fb.total_feedbacks > 0
    ? Math.round((fb.positive_count / fb.total_feedbacks) * 100)
    : 0
  const negativePct = fb && fb.total_feedbacks > 0
    ? 100 - positivePct
    : 0

  const series = data?.messages_per_day || []
  const maxCount = series.reduce((m, d) => Math.max(m, d.count), 0) || 1

  return (
    <div className="analytics-page">
      <header className="analytics-header">
        <button onClick={onBack} className="analytics-back">
          <ArrowLeft size={18} />
          Back
        </button>
        <div className="analytics-title">
          <h1>{bot.name}</h1>
          <p>Analytics</p>
        </div>
      </header>

      <main className="analytics-main">
        {isLoading ? (
          <div className="analytics-loading">Loading analytics...</div>
        ) : error ? (
          <div className="analytics-error">{error}</div>
        ) : !data ? (
          <div className="analytics-empty">No data</div>
        ) : (
          <>
            <section className="analytics-cards">
              <div className="analytics-card">
                <div className="analytics-card-icon">
                  <MessagesSquare size={20} />
                </div>
                <div className="analytics-card-value">{data.total_conversations}</div>
                <div className="analytics-card-label">Conversations</div>
              </div>
              <div className="analytics-card">
                <div className="analytics-card-icon">
                  <MessageSquare size={20} />
                </div>
                <div className="analytics-card-value">{data.total_messages}</div>
                <div className="analytics-card-label">Messages total</div>
              </div>
              <div className="analytics-card">
                <div className="analytics-card-icon">
                  <Activity size={20} />
                </div>
                <div className="analytics-card-value">{data.assistant_messages}</div>
                <div className="analytics-card-label">Assistant replies</div>
              </div>
              <div className="analytics-card">
                <div className="analytics-card-icon">
                  <ThumbsUp size={20} />
                </div>
                <div className="analytics-card-value">{fb?.total_feedbacks ?? 0}</div>
                <div className="analytics-card-label">Ratings submitted</div>
              </div>
            </section>

            <section className="analytics-block">
              <h2>Feedback</h2>
              {fb && fb.total_feedbacks > 0 ? (
                <>
                  <div className="feedback-summary">
                    <div className="feedback-summary-item positive">
                      <ThumbsUp size={16} />
                      <span>{fb.positive_count}</span>
                      <small>({positivePct}%)</small>
                    </div>
                    <div className="feedback-summary-item negative">
                      <ThumbsDown size={16} />
                      <span>{fb.negative_count}</span>
                      <small>({negativePct}%)</small>
                    </div>
                  </div>
                  <div className="feedback-bar">
                    <div
                      className="feedback-bar-positive"
                      style={{ width: `${positivePct}%` }}
                      title={`Positive: ${positivePct}%`}
                    />
                    <div
                      className="feedback-bar-negative"
                      style={{ width: `${negativePct}%` }}
                      title={`Negative: ${negativePct}%`}
                    />
                  </div>
                </>
              ) : (
                <div className="analytics-empty-inline">No feedback yet</div>
              )}
            </section>

            <section className="analytics-block">
              <h2>User messages per day (last 30 days)</h2>
              {series.length === 0 ? (
                <div className="analytics-empty-inline">No user messages in the last 30 days</div>
              ) : (
                <div className="chart">
                  {series.map(d => (
                    <div key={d.date} className="chart-bar-wrap" title={`${d.date}: ${d.count}`}>
                      <div
                        className="chart-bar"
                        style={{ height: `${(d.count / maxCount) * 100}%` }}
                      >
                        <span className="chart-bar-value">{d.count}</span>
                      </div>
                      <div className="chart-bar-label">{d.date.slice(5)}</div>
                    </div>
                  ))}
                </div>
              )}
            </section>
          </>
        )}
      </main>
    </div>
  )
}

export default Analytics

import React, { useEffect, useState } from 'react'
import { ArrowLeft, Trash2, ShieldCheck, Shield, Users, Bot, MessageSquare, BarChart2 } from 'lucide-react'
import { adminAPI } from '../api/client'
import Pagination from './Pagination'
import ThemeToggle from './ThemeToggle'
import { usePagination } from '../hooks/usePagination'
import './AdminPanel.css'

// AdminPanel — platform-wide user/bot management. Visibility is gated in
// App.jsx (only rendered for users with role='admin'); the backend also
// enforces role on each endpoint, so a user manually navigating to /admin
// without admin rights gets 403s and an empty panel.
function AdminPanel({ user, onBack }) {
  const [tab, setTab] = useState('overview') // 'overview' | 'users' | 'bots'
  const [stats, setStats] = useState(null)
  const [users, setUsers] = useState([])
  const [bots, setBots] = useState([])
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)
  const usersPage = usePagination({ page: 1, limit: 20 })
  const botsPage = usePagination({ page: 1, limit: 20 })

  // Stats are cheap (4 COUNTs) — refetch each time the overview tab opens
  // so the operator never sees stale numbers after a bot/user delete.
  useEffect(() => {
    if (tab !== 'overview') return
    adminAPI.getStats()
      .then(setStats)
      .catch(err => setError(err.message || 'Failed to load stats'))
  }, [tab])

  useEffect(() => {
    if (tab !== 'users') return
    setBusy(true)
    adminAPI.listUsers({ page: usersPage.page, limit: usersPage.limit })
      .then(data => {
        setUsers(data.items || [])
        usersPage.setMeta(data.pagination || null)
      })
      .catch(err => setError(err.message || 'Failed to load users'))
      .finally(() => setBusy(false))
  }, [tab, usersPage.page, usersPage.limit])

  useEffect(() => {
    if (tab !== 'bots') return
    setBusy(true)
    adminAPI.listBots({ page: botsPage.page, limit: botsPage.limit })
      .then(data => {
        setBots(data.items || [])
        botsPage.setMeta(data.pagination || null)
      })
      .catch(err => setError(err.message || 'Failed to load bots'))
      .finally(() => setBusy(false))
  }, [tab, botsPage.page, botsPage.limit])

  const handleToggleAdmin = async (target) => {
    const next = target.role === 'admin' ? 'user' : 'admin'
    if (target.id === user.id && next === 'user') {
      alert("You can't demote yourself.")
      return
    }
    if (!confirm(`${next === 'admin' ? 'Promote' : 'Demote'} ${target.email}?`)) return
    try {
      await adminAPI.setUserRole(target.id, next)
      setUsers(prev => prev.map(u => u.id === target.id ? { ...u, role: next } : u))
    } catch (err) {
      alert(err.message || 'Failed to change role')
    }
  }

  const handleDeleteUser = async (target) => {
    if (target.id === user.id) {
      alert("You can't delete yourself.")
      return
    }
    if (!confirm(`Delete ${target.email}? This will remove all their bots, conversations, and messages.`)) return
    try {
      await adminAPI.deleteUser(target.id)
      setUsers(prev => prev.filter(u => u.id !== target.id))
    } catch (err) {
      alert(err.message || 'Failed to delete user')
    }
  }

  const handleDeleteBot = async (botRow) => {
    if (!confirm(`Delete bot "${botRow.name}"? All conversations and documents will be removed.`)) return
    try {
      await adminAPI.deleteBot(botRow.id)
      setBots(prev => prev.filter(b => b.id !== botRow.id))
    } catch (err) {
      alert(err.message || 'Failed to delete bot')
    }
  }

  return (
    <div className="admin-panel">
      <header className="admin-header">
        <button onClick={onBack} className="back-btn">
          <ArrowLeft size={18} />
          <span>Back</span>
        </button>
        <h1><ShieldCheck size={22} style={{ verticalAlign: 'middle', marginRight: 8 }} />Admin Panel</h1>
        <ThemeToggle className="admin-theme-toggle" />
      </header>

      <nav className="admin-tabs">
        <button className={tab === 'overview' ? 'active' : ''} onClick={() => setTab('overview')}>
          <BarChart2 size={16} /> Overview
        </button>
        <button className={tab === 'users' ? 'active' : ''} onClick={() => setTab('users')}>
          <Users size={16} /> Users
        </button>
        <button className={tab === 'bots' ? 'active' : ''} onClick={() => setTab('bots')}>
          <Bot size={16} /> Bots
        </button>
      </nav>

      {error && <div className="error-message">{error}</div>}

      {tab === 'overview' && (
        <div className="admin-stats-grid">
          <StatCard icon={<Users size={20} />} label="Users" value={stats?.total_users} />
          <StatCard icon={<Bot size={20} />} label="Bots" value={stats?.total_bots} />
          <StatCard icon={<MessageSquare size={20} />} label="Conversations" value={stats?.total_conversations} />
          <StatCard icon={<MessageSquare size={20} />} label="Messages" value={stats?.total_messages} />
        </div>
      )}

      {tab === 'users' && (
        <div className="admin-table-wrapper">
          {busy ? <div className="loading">Loading users...</div> : (
            <>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Email</th>
                    <th>Name</th>
                    <th>Role</th>
                    <th>Joined</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {users.map(u => (
                    <tr key={u.id}>
                      <td>{u.email}</td>
                      <td>{u.name || '—'}</td>
                      <td>
                        <span className={`role-badge ${u.role}`}>{u.role}</span>
                      </td>
                      <td>{new Date(u.created_at).toLocaleDateString()}</td>
                      <td className="admin-actions">
                        <button
                          onClick={() => handleToggleAdmin(u)}
                          className="action-btn"
                          title={u.role === 'admin' ? 'Demote to user' : 'Promote to admin'}
                          disabled={u.id === user.id && u.role === 'admin'}
                        >
                          {u.role === 'admin' ? <Shield size={16} /> : <ShieldCheck size={16} />}
                        </button>
                        <button
                          onClick={() => handleDeleteUser(u)}
                          className="action-btn danger"
                          title="Delete user"
                          disabled={u.id === user.id}
                        >
                          <Trash2 size={16} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <Pagination meta={usersPage.meta} onPageChange={usersPage.setPage} />
            </>
          )}
        </div>
      )}

      {tab === 'bots' && (
        <div className="admin-table-wrapper">
          {busy ? <div className="loading">Loading bots...</div> : (
            <>
              <table className="admin-table">
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Owner</th>
                    <th>Status</th>
                    <th>Created</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {bots.map(b => (
                    <tr key={b.id}>
                      <td>{b.name}</td>
                      <td>{b.owner_email}</td>
                      <td>
                        <span className={`status-badge ${b.is_active ? 'active' : 'inactive'}`}>
                          {b.is_active ? 'Active' : 'Inactive'}
                        </span>
                      </td>
                      <td>{new Date(b.created_at).toLocaleDateString()}</td>
                      <td className="admin-actions">
                        <button
                          onClick={() => handleDeleteBot(b)}
                          className="action-btn danger"
                          title="Delete bot"
                        >
                          <Trash2 size={16} />
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              <Pagination meta={botsPage.meta} onPageChange={botsPage.setPage} />
            </>
          )}
        </div>
      )}
    </div>
  )
}

function StatCard({ icon, label, value }) {
  return (
    <div className="stat-card">
      <div className="stat-icon">{icon}</div>
      <div className="stat-body">
        <div className="stat-value">{value ?? '—'}</div>
        <div className="stat-label">{label}</div>
      </div>
    </div>
  )
}

export default AdminPanel

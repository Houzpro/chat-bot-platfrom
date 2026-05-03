const API_BASE = '/api/v1'

// Build `?page=N&limit=M&search=...&...` for paginated/filtered endpoints.
// Null/undefined/empty values are dropped so the backend applies its defaults
// and pagination doesn't get polluted with `search=` when the user cleared
// the box.
const buildQuery = (obj = {}) => {
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(obj)) {
    if (v == null || v === '') continue
    params.set(k, v)
  }
  const s = params.toString()
  return s ? `?${s}` : ''
}

// Token management
export const getToken = () => localStorage.getItem('token')
export const setToken = (token) => localStorage.setItem('token', token)
export const removeToken = () => localStorage.removeItem('token')

// Helper for API calls with auth
const apiCall = async (endpoint, options = {}) => {
  const token = getToken()
  const headers = {
    'Content-Type': 'application/json',
    ...options.headers,
  }
  
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  })

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Network error' }))
    throw new Error(error.error || `HTTP ${response.status}`)
  }

  return response.json()
}

// Config API — single source of truth for limits driven by backend .env
export const configAPI = {
  getDefaults: async () => {
    const response = await fetch(`${API_BASE}/config/defaults`)
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }
    return response.json()
  },
}

// Auth API
export const authAPI = {
  register: async (email, password, name) => {
    return apiCall('/auth/register', {
      method: 'POST',
      body: JSON.stringify({ email, password, name }),
    })
  },

  login: async (email, password) => {
    return apiCall('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    })
  },

  me: async () => {
    return apiCall('/auth/me')
  },
}

// Bots API
export const botsAPI = {
  create: async (botData) => {
    return apiCall('/bots', {
      method: 'POST',
      body: JSON.stringify(botData),
    })
  },

  // Returns the pagination envelope: { items, pagination: { page, limit, total, total_pages, has_next, has_prev } }
  getMyBots: async (params) => {
    return apiCall(`/bots${buildQuery(params)}`)
  },

  getBot: async (id) => {
    return apiCall(`/bots/${id}`)
  },

  update: async (id, botData) => {
    return apiCall(`/bots/${id}`, {
      method: 'PUT',
      body: JSON.stringify(botData),
    })
  },

  delete: async (id) => {
    return apiCall(`/bots/${id}`, {
      method: 'DELETE',
    })
  },

  // Returns the pagination envelope. Pass { page, limit } to control paging.
  getDocuments: async (id, params) => {
    return apiCall(`/bots/${id}/documents${buildQuery(params)}`)
  },

  deleteDocument: async (botId, docId) => {
    return apiCall(`/bots/${botId}/documents/${docId}`, {
      method: 'DELETE',
    })
  },

  uploadDocument: async (id, file) => {
    const token = getToken()
    const formData = new FormData()
    formData.append('file', file)

    const response = await fetch(`${API_BASE}/bots/${id}/documents/upload`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${token}`,
      },
      body: formData,
    })

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Upload failed' }))
      throw new Error(error.error || `HTTP ${response.status}`)
    }

    return response.json()
  },
}

// Public chat API (no auth required)
export const publicChatAPI = {
  sendMessage: async (botId, query, settings = {}) => {
    const response = await fetch(`${API_BASE}/chat/public/${botId}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify({
        query,
        limit: 60,
        ...settings,
      }),
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    return response // Return response for streaming
  },
}

// Conversations API
export const conversationsAPI = {
  create: async (botId, title) => {
    return apiCall('/conversations', {
      method: 'POST',
      body: JSON.stringify({ bot_id: botId, title: title || '' }),
    })
  },

  // Returns the pagination envelope.
  getByBot: async (botId, params) => {
    return apiCall(`/bots/${botId}/conversations${buildQuery(params)}`)
  },

  get: async (convId) => {
    return apiCall(`/conversations/${convId}`)
  },

  delete: async (convId) => {
    return apiCall(`/conversations/${convId}`, {
      method: 'DELETE',
    })
  },
}

// Feedback API
export const feedbackAPI = {
  submit: async (messageId, rating) => {
    return apiCall(`/messages/${messageId}/feedback`, {
      method: 'POST',
      body: JSON.stringify({ rating }),
    })
  },

  getStats: async (botId) => {
    return apiCall(`/bots/${botId}/feedback/stats`)
  },
}

// Analytics API
export const analyticsAPI = {
  getBotAnalytics: async (botId) => {
    return apiCall(`/bots/${botId}/analytics`)
  },
}

// Collaborators API — bot sharing. Owner-only management; collaborators see
// shared bots in their bot list (with role flag) but cannot manage other
// collaborators themselves.
export const collaboratorsAPI = {
  list: async (botId) => apiCall(`/bots/${botId}/collaborators`),
  add: async (botId, email, role) =>
    apiCall(`/bots/${botId}/collaborators`, {
      method: 'POST',
      body: JSON.stringify({ email, role }),
    }),
  updateRole: async (botId, userId, role) =>
    apiCall(`/bots/${botId}/collaborators/${userId}`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),
  remove: async (botId, userId) =>
    apiCall(`/bots/${botId}/collaborators/${userId}`, { method: 'DELETE' }),
}

// Admin API — restricted to users with role='admin'. The middleware returns
// 403 for everyone else, which the UI treats as "hide the admin panel link."
export const adminAPI = {
  getStats: async () => apiCall('/admin/stats'),
  listUsers: async (params) => apiCall(`/admin/users${buildQuery(params)}`),
  setUserRole: async (userId, role) =>
    apiCall(`/admin/users/${userId}/role`, {
      method: 'PUT',
      body: JSON.stringify({ role }),
    }),
  deleteUser: async (userId) =>
    apiCall(`/admin/users/${userId}`, { method: 'DELETE' }),
  listBots: async (params) => apiCall(`/admin/bots${buildQuery(params)}`),
  deleteBot: async (botId) =>
    apiCall(`/admin/bots/${botId}`, { method: 'DELETE' }),
}

// Public Feedback API (no auth required)
export const publicFeedbackAPI = {
  submit: async (messageId, rating) => {
    const response = await fetch(`${API_BASE}/public/messages/${messageId}/feedback`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ rating }),
    })
    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: 'Network error' }))
      throw new Error(error.error || `HTTP ${response.status}`)
    }
    return response.json()
  },
}

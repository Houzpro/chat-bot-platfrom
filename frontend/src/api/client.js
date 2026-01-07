const API_BASE = 'http://localhost:8080/api/v1'

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

  getMyBots: async () => {
    return apiCall('/bots')
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

  getDocuments: async (id) => {
    return apiCall(`/bots/${id}/documents`)
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
        limit: 3,
        ...settings,
      }),
    })

    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`)
    }

    return response // Return response for streaming
  },
}

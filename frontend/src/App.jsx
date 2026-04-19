import React, { useState, useEffect } from 'react'
import Login from './components/Login'
import Dashboard from './components/Dashboard'
import PublicChat from './components/PublicChat'
import { useTheme } from './hooks/useTheme'
import './App.css'

const API_BASE = '/api/v1'

// Parse the current URL into a route. Kept inline (no router library) so the
// bundle stays tiny and we don't introduce new deps. Three shapes exist:
//   /public/:botId → public chat (no auth, no dashboard back-button)
//   /chat/:botId   → authenticated chat opened from the dashboard
//   anything else  → authenticated app root (login or dashboard)
const parseRoute = () => {
  if (typeof window === 'undefined') return { name: 'app' }
  const path = window.location.pathname
  const pub = path.match(/^\/public\/([^/]+)\/?$/)
  if (pub) return { name: 'public-chat', botId: pub[1] }
  const chat = path.match(/^\/chat\/([^/]+)\/?$/)
  if (chat) return { name: 'app-chat', botId: chat[1] }
  const analytics = path.match(/^\/analytics\/([^/]+)\/?$/)
  if (analytics) return { name: 'app-analytics', botId: analytics[1] }
  return { name: 'app' }
}

function App() {
  // Initialize theme as early as possible so the first paint is already themed.
  useTheme()

  const [route, setRoute] = useState(parseRoute)

  // Keep `route` in sync with browser navigation (back/forward buttons).
  useEffect(() => {
    const onPop = () => setRoute(parseRoute())
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [])

  // Programmatic navigation: child components call this to change both the
  // URL and the route state in a single step. pushState alone does NOT fire
  // popstate, so we have to manually re-parse and setRoute — otherwise
  // App.jsx would keep rendering the previous route even though the address
  // bar changed. Using replace:true swaps the current entry instead of
  // adding a new one (useful for redirects after login, etc.).
  const navigate = (path, { replace = false } = {}) => {
    if (typeof window === 'undefined') return
    if (replace) {
      window.history.replaceState({}, '', path)
    } else {
      window.history.pushState({}, '', path)
    }
    setRoute(parseRoute())
  }

  const [token, setToken] = useState(null)
  const [user, setUser] = useState(null)
  const [isLoading, setIsLoading] = useState(true)

  // Skip auth check on the public chat route — those users may not be logged in,
  // and we don't want a spurious /auth/me request (or the loading screen flash)
  // on a page that deliberately works without a token.
  const isPublicRoute = route.name === 'public-chat'

  // Check for existing session on mount
  useEffect(() => {
    if (isPublicRoute) {
      setIsLoading(false)
      return
    }
    const checkAuth = async () => {
      const savedToken = localStorage.getItem('token')
      const savedUser = localStorage.getItem('user')
      
      if (savedToken && savedUser) {
        try {
          // Verify token is still valid
          const response = await fetch(`${API_BASE}/auth/me`, {
            headers: {
              'Authorization': `Bearer ${savedToken}`
            }
          })

          if (response.ok) {
            const userData = await response.json()
            setToken(savedToken)
            setUser(userData)
          } else {
            // Token invalid, clear storage
            localStorage.removeItem('token')
            localStorage.removeItem('user')
          }
        } catch (err) {
          console.error('Auth check failed:', err)
          localStorage.removeItem('token')
          localStorage.removeItem('user')
        }
      }
      setIsLoading(false)
    }

    checkAuth()
  }, [isPublicRoute])

  const handleLoginSuccess = (newToken, newUser) => {
    console.log('handleLoginSuccess called:', { newToken, newUser })
    setToken(newToken)
    setUser(newUser)
    console.log('State updated - token:', newToken ? 'exists' : 'null', 'user:', newUser ? 'exists' : 'null')
  }

  const handleLogout = () => {
    localStorage.removeItem('token')
    localStorage.removeItem('user')
    setToken(null)
    setUser(null)
  }

  // Public chat route short-circuits everything else — no auth, no dashboard.
  if (route.name === 'public-chat') {
    return <PublicChat botId={route.botId} />
  }

  if (isLoading) {
    return (
      <div className="app loading-screen">
        <div className="loader">Loading...</div>
      </div>
    )
  }

  if (!token || !user) {
    return <Login onLoginSuccess={handleLoginSuccess} />
  }

  // Authenticated section. App.jsx owns the route — Dashboard renders the list
  // and opens a bot via navigate('/chat/:id'), which updates both URL and
  // route state so F5 and Back behave the same way.
  const activeBotId = route.name === 'app-chat' ? route.botId : null
  const analyticsBotId = route.name === 'app-analytics' ? route.botId : null

  return (
    <Dashboard
      token={token}
      user={user}
      onLogout={handleLogout}
      activeBotId={activeBotId}
      analyticsBotId={analyticsBotId}
      navigate={navigate}
    />
  )
}

export default App

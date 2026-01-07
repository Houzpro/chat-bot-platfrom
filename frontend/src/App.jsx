import React, { useState, useEffect } from 'react'
import Login from './components/Login'
import Dashboard from './components/Dashboard'
import './App.css'

const API_BASE = 'http://localhost:8080/api/v1'

function App() {
  const [token, setToken] = useState(null)
  const [user, setUser] = useState(null)
  const [isLoading, setIsLoading] = useState(true)

  // Check for existing session on mount
  useEffect(() => {
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
  }, [])

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

  return <Dashboard token={token} user={user} onLogout={handleLogout} />
}

export default App

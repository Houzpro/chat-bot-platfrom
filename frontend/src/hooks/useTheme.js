import { useCallback, useEffect, useState } from 'react'

// Theme lives on <html data-theme="..."> so CSS variables in :root can switch
// atomically. Persisted in localStorage under this key.
const STORAGE_KEY = 'theme'
const VALID = ['dark', 'light']

const getInitialTheme = () => {
  if (typeof window === 'undefined') return 'dark'
  const saved = window.localStorage.getItem(STORAGE_KEY)
  if (VALID.includes(saved)) return saved
  // Respect system preference on first visit
  if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
    return 'light'
  }
  return 'dark'
}

const applyTheme = (theme) => {
  if (typeof document === 'undefined') return
  document.documentElement.setAttribute('data-theme', theme)
  // color-scheme lets the browser theme native UI (scrollbars, form controls)
  document.documentElement.style.colorScheme = theme
}

export function useTheme() {
  const [theme, setThemeState] = useState(getInitialTheme)

  useEffect(() => {
    applyTheme(theme)
    try {
      window.localStorage.setItem(STORAGE_KEY, theme)
    } catch {
      // ignore storage errors (private mode, quota)
    }
  }, [theme])

  const setTheme = useCallback((next) => {
    if (!VALID.includes(next)) return
    setThemeState(next)
  }, [])

  const toggleTheme = useCallback(() => {
    setThemeState(prev => (prev === 'dark' ? 'light' : 'dark'))
  }, [])

  return { theme, setTheme, toggleTheme }
}

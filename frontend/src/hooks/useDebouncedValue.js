import { useEffect, useState } from 'react'

// useDebouncedValue — returns `value` only after it has been stable for
// `delay` ms. Cancel in-flight timers on every change so we don't fire
// an old value after the user kept typing. Used for search inputs so
// the backend isn't hit on every keystroke.
export function useDebouncedValue(value, delay = 300) {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const t = setTimeout(() => setDebounced(value), delay)
    return () => clearTimeout(t)
  }, [value, delay])

  return debounced
}

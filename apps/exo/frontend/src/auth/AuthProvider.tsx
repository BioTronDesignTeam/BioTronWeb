import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { AUTH_APP_ID, AUTH_URL, AuthContext, type AuthState, type Operator } from './context'

// Re-check the session this often while the tab is open, so a guest session that
// expires at Eastern midnight (or a revoked operator) is noticed without a reload.
const REVALIDATE_INTERVAL_MS = 10 * 60 * 1000

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  const refresh = useCallback(async () => {
    for (let attempt = 0; ; attempt++) {
      try {
        // credentials: 'include' so the cross-origin session cookie is sent.
        const res = await fetch(`${AUTH_URL}/auth/me?app=${encodeURIComponent(AUTH_APP_ID)}`, {
          credentials: 'include',
        })
        if (res.ok) {
          setState({ status: 'authed', operator: (await res.json()) as Operator })
          return
        }
        // Only an explicit 401/403 means "not logged in".
        if (res.status === 401 || res.status === 403) {
          setState({ status: 'anon' })
          return
        }
      } catch {
        // Network/CORS error — transient, fall through to retry.
      }
      // A transient backend/tunnel failure is NOT a logout: never drop an authed
      // session to the login screen over a blip. Retry a few times (covers a hiccup
      // on initial load), then keep an authed state if we already had one.
      if (attempt >= 2) {
        setState((prev) => (prev.status === 'authed' ? prev : { status: 'anon' }))
        return
      }
      await new Promise((r) => setTimeout(r, 500 * (attempt + 1)))
    }
  }, [])

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh()
    const revalidate = () => {
      if (document.visibilityState === 'visible') void refresh()
    }
    document.addEventListener('visibilitychange', revalidate)
    window.addEventListener('focus', revalidate)
    const interval = window.setInterval(() => void refresh(), REVALIDATE_INTERVAL_MS)
    return () => {
      document.removeEventListener('visibilitychange', revalidate)
      window.removeEventListener('focus', revalidate)
      window.clearInterval(interval)
    }
  }, [refresh])

  const login = useCallback(() => {
    window.location.href = `${AUTH_URL}/auth/github/login?redirect=${encodeURIComponent(window.location.origin)}`
  }, [])

  const loginGuest = useCallback(
    async (key: string) => {
      try {
        const res = await fetch(`${AUTH_URL}/auth/guest`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ app_id: AUTH_APP_ID, key }),
        })
        if (!res.ok) return false
        await refresh()
        return true
      } catch {
        return false
      }
    },
    [refresh],
  )

  const logout = useCallback(async () => {
    try {
      const res = await fetch(`${AUTH_URL}/auth/logout`, {
        method: 'POST',
        credentials: 'include',
        // The custom header forces a CORS preflight a cross-site page can't pass,
        // and the backend requires it — together this blocks logout CSRF.
        headers: { 'X-Requested-With': 'XMLHttpRequest' },
      })
      // Only treat ourselves as logged out once the server confirms it; otherwise a
      // failed request would show "signed out" while the session cookie is still live.
      if (res.ok) setState({ status: 'anon' })
    } catch {
      // Network error — keep the current state; the session may still be valid.
    }
  }, [])

  // Memoized so the provider value keeps its identity between renders. Built
  // inline, it was a fresh object every render and every consumer re-rendered
  // even when the session had not changed. The handlers are already useCallback
  // -stable, so only `state` actually moves.
  const value = useMemo(
    () => ({ state, login, loginGuest, logout, refresh }),
    [state, login, loginGuest, logout, refresh],
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

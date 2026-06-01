import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { API_URL, AuthContext, type AuthState, type Operator } from './context'

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: 'loading' })

  const refresh = useCallback(async () => {
    try {
      // credentials: 'include' so the cross-origin session cookie is sent.
      const res = await fetch(`${API_URL}/auth/me`, { credentials: 'include' })
      if (res.ok) {
        setState({ status: 'authed', operator: (await res.json()) as Operator })
      } else {
        setState({ status: 'anon' })
      }
    } catch {
      setState({ status: 'anon' })
    }
  }, [])

  useEffect(() => {
    // Fetch the session once on mount — a valid external-sync effect; setState
    // runs after the await, not synchronously.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void refresh()
  }, [refresh])

  const login = useCallback(() => {
    window.location.href = `${API_URL}/auth/github/login`
  }, [])

  const loginGuest = useCallback(
    async (key: string) => {
      try {
        const res = await fetch(`${API_URL}/auth/guest`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ key }),
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
      await fetch(`${API_URL}/auth/logout`, { method: 'POST', credentials: 'include' })
    } finally {
      setState({ status: 'anon' })
    }
  }, [])

  return (
    <AuthContext.Provider value={{ state, login, loginGuest, logout, refresh }}>{children}</AuthContext.Provider>
  )
}

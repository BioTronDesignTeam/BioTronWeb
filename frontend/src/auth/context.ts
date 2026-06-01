import { createContext, useContext } from 'react'

export const API_URL = import.meta.env.VITE_API_URL ?? 'http://localhost:8080'

export interface Operator {
  github_id: number
  login: string
  name: string
  avatar_url: string
}

export type AuthState =
  | { status: 'loading' }
  | { status: 'anon' }
  | { status: 'authed'; operator: Operator }

export interface AuthContextValue {
  state: AuthState
  login: () => void
  loginGuest: (key: string) => Promise<boolean>
  logout: () => Promise<void>
  refresh: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue | null>(null)

export function useAuth() {
  const ctx = useContext(AuthContext)
  if (!ctx) {
    throw new Error('useAuth must be used within an AuthProvider')
  }
  return ctx
}

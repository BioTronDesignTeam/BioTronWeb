const authUrl = import.meta.env.VITE_AUTH_URL
const apiUrl = import.meta.env.VITE_API_URL
if (import.meta.env.PROD && !authUrl) {
  throw new Error('VITE_AUTH_URL must be set for production builds')
}
if (import.meta.env.PROD && !apiUrl) {
  throw new Error('VITE_API_URL must be set for production builds')
}

export const AUTH_URL = authUrl ?? 'http://localhost:8080'
export const API_URL = apiUrl ?? 'http://localhost:8081'
export const AUTH_APP_ID = 'exo-gui'
export const PRODUCT_NAME = 'Exo GUI'
// Guests hold live and historical. A member needs at least one grant to see
// the app at all; without one, the page says Access denied.
export const EXO_PERMISSIONS = ['live', 'historical', 'commands'] as const

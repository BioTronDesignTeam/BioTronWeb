import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { SessionProvider } from '@biotron/style'
import '@biotron/style/styles.css'
import './index.css'
import App from './App.tsx'
import { AUTH_APP_ID, AUTH_URL, EXO_PERMISSIONS } from './config'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <SessionProvider authUrl={AUTH_URL} appId={AUTH_APP_ID} permissions={EXO_PERMISSIONS}>
      <App />
    </SessionProvider>
  </StrictMode>,
)

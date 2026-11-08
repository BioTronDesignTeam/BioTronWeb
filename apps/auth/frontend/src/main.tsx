import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { SessionProvider } from '@biotron/style';
import '@biotron/style/styles.css';
import { API_BASE } from './api';
import App from './App';
import './index.css';

// No app id: Auth's own pages refuse guests, and a guest's /auth/me without
// one answers 403, which reads as signed out.
createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <SessionProvider authUrl={API_BASE}>
      <App />
    </SessionProvider>
  </StrictMode>,
);

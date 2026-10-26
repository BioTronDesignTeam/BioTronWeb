import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { SessionProvider } from '@biotron/style';
import { App } from './App';
import { AUTH_URL } from './api';
import './index.css';
import '@biotron/style/styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <SessionProvider authUrl={AUTH_URL} appId="calendar" permissions={['write']}>
      <App />
    </SessionProvider>
  </StrictMode>,
);

import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AuthScreen } from './index';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <AuthScreen
      productName="BioTron component preview"
      description="One shared authentication surface, theme control, account menu, and visual language for every BioTron tool."
      action={{ label: 'Sign in with GitHub', href: '#preview', icon: 'github' }}
      guestAccess={{ onSubmit: async () => false }}
      footer="Palette: #16033c · #160b6c · #aedbfc · #3050b0 · #ffffff"
    />
  </StrictMode>,
);

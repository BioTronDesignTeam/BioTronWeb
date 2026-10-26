import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { AuthScreen, Brand, Button, ThemeToggle, UserMenu, authStrings } from './index';
import './styles.css';

const showControls = new URLSearchParams(window.location.search).has('controls');

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {showControls ? (
      <main className="biotron-preview">
        <Brand />
        <p className="biotron-eyebrow">Shared controls</p>
        <h1>BioTron interface primitives</h1>
        <div className="biotron-preview__toolbar">
          <ThemeToggle />
          <UserMenu
            user={{ name: 'Ada Lovelace', login: 'ada', detail: '@ada · Manager' }}
            onLogout={() => undefined}
          />
        </div>
        <div className="biotron-preview__actions">
          <Button>Primary action</Button>
          <Button tone="neutral">Neutral action</Button>
        </div>
      </main>
    ) : (
      <AuthScreen
        productName="BioTron component preview"
        action={{ label: authStrings.signInWithGitHub, href: '#preview', icon: 'github' }}
        guestAccess={{ onSubmit: async () => false }}
        footer="Palette: #16033c · #160b6c · #aedbfc · #3050b0 · #ffffff"
      />
    )}
  </StrictMode>,
);

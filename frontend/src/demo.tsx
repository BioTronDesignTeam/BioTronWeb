import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Button } from './index';
import './styles.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <main className="biotron-preview">
      <p className="biotron-eyebrow">BioTron UW</p>
      <h1>Shared interface primitives</h1>
      <div className="biotron-preview__actions">
        <Button>Primary action</Button>
        <Button tone="neutral">Neutral action</Button>
      </div>
    </main>
  </StrictMode>,
);

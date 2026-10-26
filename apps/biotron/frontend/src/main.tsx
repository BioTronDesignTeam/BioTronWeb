import React from 'react';
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import App from './App';

import '@fontsource/space-grotesk/500.css';
import '@fontsource/space-grotesk/600.css';
import '@fontsource/space-grotesk/700.css';
import '@fontsource/inter/400.css';
import '@fontsource/inter/500.css';
import '@fontsource/jetbrains-mono/400.css';
import '@fontsource/jetbrains-mono/500.css';
import '@biotron/style/styles.css';

import './styles/tokens.css';
import './styles/globals.css';
import './styles/ui.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    {/* v7 turns the former v7_startTransition and v7_relativeSplatPath opt-ins
        into plain behaviour, so the future flags are gone. */}
    <BrowserRouter>
      <App />
    </BrowserRouter>
  </React.StrictMode>,
);

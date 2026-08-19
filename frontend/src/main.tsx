import React from 'react';
import { createRoot } from 'react-dom/client';
import Root from './Root';
import './index.css';
import { TOKEN_PARAM } from './lib/http';
import { startSaving } from './lib/persist';

function stripToken(): void {
  const injected = window as unknown as { __SPINOZA_TOKEN__?: string };
  if (typeof injected.__SPINOZA_TOKEN__ !== 'string' || injected.__SPINOZA_TOKEN__ === '') {
    return;
  }
  const params = new URLSearchParams(window.location.search);
  if (!params.has(TOKEN_PARAM)) {
    return;
  }
  params.delete(TOKEN_PARAM);
  const query = params.toString();
  let search = '';
  if (query !== '') {
    search = `?${query}`;
  }
  window.history.replaceState(null, '', window.location.pathname + search + window.location.hash);
}

const el = document.getElementById('root');
if (!el) {
  throw new Error('root element missing');
}

stripToken();
startSaving();

createRoot(el).render(
  <React.StrictMode>
    <Root />
  </React.StrictMode>,
);

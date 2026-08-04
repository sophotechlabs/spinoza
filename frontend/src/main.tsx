import React from 'react';
import { createRoot } from 'react-dom/client';
import App from './App';
import ErrorBoundary from './components/ErrorBoundary';
import './lib/monaco';
import './index.css';

const el = document.getElementById('root');
if (!el) {
  throw new Error('root element missing');
}

createRoot(el).render(
  <React.StrictMode>
    <ErrorBoundary label="Spinoza">
      <App />
    </ErrorBoundary>
  </React.StrictMode>,
);

import React from 'react';
import { createRoot } from 'react-dom/client';
import Root from './Root';
import './index.css';
import { startSaving } from './lib/persist';

const el = document.getElementById('root');
if (!el) {
  throw new Error('root element missing');
}

startSaving();

createRoot(el).render(
  <React.StrictMode>
    <Root />
  </React.StrictMode>,
);

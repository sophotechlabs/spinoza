import { TOKEN_PARAM, authToken } from './http';
import { onCluster } from './cluster';

function override(): string | null {
  const w = window as unknown as { __SPINOZA_WS_BASE__?: string };
  if (typeof w.__SPINOZA_WS_BASE__ === 'string') {
    return w.__SPINOZA_WS_BASE__;
  }
  return null;
}

function origin(): string {
  const base = override();
  if (base !== null) {
    return base;
  }
  let proto = 'ws';
  if (location.protocol === 'https:') {
    proto = 'wss';
  }
  return `${proto}://${location.host}`;
}

function separator(path: string): string {
  if (path.includes('?')) {
    return '&';
  }
  return '?';
}

export function wsURL(path: string): string {
  const named = onCluster(path);
  const token = authToken();
  if (token === null) {
    return `${origin()}${named}`;
  }
  const param = `${TOKEN_PARAM}=${encodeURIComponent(token)}`;
  return `${origin()}${named}${separator(named)}${param}`;
}

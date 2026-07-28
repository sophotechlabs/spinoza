function override(): string | null {
  const w = window as unknown as { __SPINOZA_WS_BASE__?: string };
  if (typeof w.__SPINOZA_WS_BASE__ === 'string') {
    return w.__SPINOZA_WS_BASE__;
  }
  return null;
}

export function wsURL(path: string): string {
  const base = override();
  if (base !== null) {
    return `${base}${path}`;
  }
  let proto = 'ws';
  if (location.protocol === 'https:') {
    proto = 'wss';
  }
  return `${proto}://${location.host}${path}`;
}

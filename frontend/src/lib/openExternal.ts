interface WailsWindow {
  runtime?: { BrowserOpenURL?: (url: string) => void };
}

export function forwardURL(localPort: number): string {
  return `http://127.0.0.1:${localPort}`;
}

export function openExternal(url: string): void {
  const wails = window as unknown as WailsWindow;
  const open = wails.runtime?.BrowserOpenURL;
  if (open !== undefined) {
    open(url);
    return;
  }
  window.open(url, '_blank', 'noreferrer');
}

import { BASE_URL } from './paths';

const REOPEN_MS = 250;

const CLOSE_WAIT_MS = 2000;

export interface Held {
  close: () => Promise<void>;
}

function socketURL(baseURL: string, token: string): string {
  const origin = baseURL.replace(/^http/, 'ws');
  return `${origin}/ws?view=browser&token=${encodeURIComponent(token)}`;
}

export function hold(baseURL: string, token: string): Held {
  let wanted = true;
  let socket: WebSocket | null = null;
  let reopen: ReturnType<typeof setTimeout> | null = null;

  const open = () => {
    reopen = null;
    if (!wanted) {
      return;
    }
    const next = new WebSocket(socketURL(baseURL, token));
    socket = next;
    next.addEventListener('close', () => {
      if (!wanted) {
        return;
      }
      reopen = setTimeout(open, REOPEN_MS);
    });
    next.addEventListener('error', () => {
      next.close();
    });
  };
  open();

  return {
    close: async () => {
      wanted = false;
      if (reopen !== null) {
        clearTimeout(reopen);
        reopen = null;
      }
      const held = socket;
      socket = null;
      if (held === null || held.readyState === WebSocket.CLOSED) {
        return;
      }
      await new Promise<void>((resolve) => {
        const settle = () => {
          clearTimeout(giveUp);
          resolve();
        };
        const giveUp = setTimeout(settle, CLOSE_WAIT_MS);
        held.addEventListener('close', settle, { once: true });
        held.addEventListener('error', settle, { once: true });
        held.close();
      });
    },
  };
}

export function holdMain(token: string): Held {
  return hold(BASE_URL, token);
}

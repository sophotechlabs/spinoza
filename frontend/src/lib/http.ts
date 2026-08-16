import { expireSession } from '../store/session';

export const REQUEST_TIMEOUT_MS = 15000;
export const SLOW_REQUEST_TIMEOUT_MS = 120000;

export const TIMEOUT_MESSAGE = 'the backend did not answer in time';

const UNAUTHORIZED = 401;

export const TOKEN_HEADER = 'X-Spinoza-Token';
export const TOKEN_PARAM = 'token';

export interface RequestOptions extends RequestInit {
  timeoutMs?: number;
}

function injectedToken(): string | null {
  const w = window as unknown as { __SPINOZA_TOKEN__?: string };
  if (typeof w.__SPINOZA_TOKEN__ === 'string' && w.__SPINOZA_TOKEN__ !== '') {
    return w.__SPINOZA_TOKEN__;
  }
  return null;
}

export function authToken(): string | null {
  const injected = injectedToken();
  if (injected !== null) {
    return injected;
  }
  const fromURL = new URLSearchParams(location.search).get(TOKEN_PARAM);
  if (fromURL !== null && fromURL !== '') {
    return fromURL;
  }
  return null;
}

function withToken(init: RequestInit): RequestInit {
  const token = authToken();
  if (token === null) {
    return init;
  }
  const headers = new Headers(init.headers);
  headers.set(TOKEN_HEADER, token);
  return { ...init, headers };
}

function timedOut(err: unknown): boolean {
  if (err instanceof Error) {
    return err.name === 'TimeoutError';
  }
  return false;
}

export async function request(url: string, options: RequestOptions = {}): Promise<Response> {
  const { timeoutMs, ...init } = options;
  let limit = REQUEST_TIMEOUT_MS;
  if (timeoutMs !== undefined) {
    limit = timeoutMs;
  }
  try {
    const response = await fetch(url, { ...withToken(init), signal: AbortSignal.timeout(limit) });
    if (response.status === UNAUTHORIZED) {
      expireSession();
    }
    return response;
  } catch (err: unknown) {
    if (timedOut(err)) {
      throw new Error(TIMEOUT_MESSAGE);
    }
    throw err;
  }
}

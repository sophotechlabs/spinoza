export const REQUEST_TIMEOUT_MS = 15000;
export const SLOW_REQUEST_TIMEOUT_MS = 120000;

export const TIMEOUT_MESSAGE = 'the backend did not answer in time';

export interface RequestOptions extends RequestInit {
  timeoutMs?: number;
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
    return await fetch(url, { ...init, signal: AbortSignal.timeout(limit) });
  } catch (err: unknown) {
    if (timedOut(err)) {
      throw new Error(TIMEOUT_MESSAGE);
    }
    throw err;
  }
}

import { request } from './http';
import { asRecord, asString } from './wire';

export const FRONTEND_VERSION: string = __SPINOZA_VERSION__;

export async function fetchBackendVersion(): Promise<string> {
  const response = await request('/api/version');
  if (!response.ok) {
    throw new Error(`version request failed with status ${String(response.status)}`);
  }
  return asString(asRecord(await response.json()).version);
}

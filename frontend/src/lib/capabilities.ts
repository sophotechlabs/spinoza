import type { Capabilities } from './types';
import { request } from './http';
import { parseCapabilities } from './parse';

export async function fetchCapabilities(): Promise<Capabilities> {
  const response = await request('/api/capabilities');
  if (!response.ok) {
    throw new Error(`capabilities request failed with status ${response.status}`);
  }
  return parseCapabilities(await response.json());
}

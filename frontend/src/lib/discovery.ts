import type { Category } from './types';

export async function fetchResources(): Promise<Category[]> {
  const response = await fetch('/api/resources');
  if (!response.ok) {
    throw new Error(`discovery request failed with status ${response.status}`);
  }
  const data = (await response.json()) as Category[];
  return data;
}

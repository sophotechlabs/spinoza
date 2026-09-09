import type { Transcripts } from './types';
import { request } from './http';
import { failure } from './object';

export async function fetchTranscripts(): Promise<Transcripts> {
  const response = await request('/api/transcripts');
  if (!response.ok) {
    throw await failure(response, `recorded sessions failed with status ${response.status}`);
  }
  return (await response.json()) as Transcripts;
}

export async function fetchTranscript(id: string): Promise<string> {
  const response = await request(`/api/transcripts/text?id=${encodeURIComponent(id)}`);
  if (!response.ok) {
    throw await failure(response, `that session failed with status ${response.status}`);
  }
  return response.text();
}

export function sizeText(bytes: number): string {
  if (bytes >= 1024 * 1024) {
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }
  if (bytes >= 1024) {
    return `${String(Math.round(bytes / 1024))} kB`;
  }
  return `${String(bytes)} bytes`;
}

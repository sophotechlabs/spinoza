import type { UpdateStatus } from './types';
import { copyText } from './clipboard';
import { failure } from './object';
import { parseUpdateStatus } from './parse';
import { request } from './http';
import { askToast } from '../store/toasts';

export async function fetchUpdateStatus(): Promise<UpdateStatus> {
  const response = await request('/api/update');
  if (!response.ok) {
    throw await failure(response, `update check failed with status ${response.status}`);
  }
  return parseUpdateStatus(await response.json());
}

export function updateMessage(status: UpdateStatus): string {
  return `Spinoza ${status.latest} is out. You are running ${status.current}.`;
}

export async function announceUpdate(): Promise<void> {
  let status: UpdateStatus;
  try {
    status = await fetchUpdateStatus();
  } catch {
    return;
  }
  if (!status.available) {
    return;
  }
  if (status.command === undefined || status.command === '') {
    return;
  }
  const command = status.command;
  askToast(updateMessage(status), {
    label: 'Copy install command',
    run: () => {
      void copyText('the install command', command);
    },
  });
}

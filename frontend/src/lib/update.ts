import type { UpdateResult, UpdateStatus } from './types';
import { copyText } from './clipboard';
import { failure } from './object';
import { parseUpdateResult, parseUpdateStatus } from './parse';
import { request } from './http';
import { askToast } from '../store/toasts';

const UPDATE_PATH = '/api/update';

const INSTALL_TIMEOUT_MS = 300000;

export async function fetchUpdateStatus(): Promise<UpdateStatus> {
  const response = await request(UPDATE_PATH);
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

export async function installUpdate(): Promise<UpdateResult> {
  const response = await request(UPDATE_PATH, {
    method: 'POST',
    timeoutMs: INSTALL_TIMEOUT_MS,
  });
  if (!response.ok) {
    throw await failure(response, `the update failed with status ${response.status}`);
  }
  return parseUpdateResult(await response.json());
}

export function updateFailure(err: unknown): string {
  if (err instanceof Error) {
    return err.message;
  }
  return 'the update failed';
}

export function updateOutcome(result: UpdateResult): string {
  if (result.updated) {
    return `Updated to ${String(result.latest)}. Restart spinoza to finish.`;
  }
  if (result.command !== undefined && result.command !== '') {
    return `This build cannot replace itself. Run: ${result.command}`;
  }
  if (result.reason !== undefined && result.reason !== '') {
    return result.reason;
  }
  return `${result.current} is the newest release.`;
}

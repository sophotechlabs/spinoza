import { notifyError, notifyOk } from '../store/toasts';

export async function copyText(what: string, text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text);
  } catch {
    notifyError(`Could not copy ${what}`);
    return;
  }
  notifyOk(`Copied ${what}`);
}

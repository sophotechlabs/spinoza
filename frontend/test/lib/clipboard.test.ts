import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { copyText } from '../../src/lib/clipboard';
import { useToastsStore } from '../../src/store/toasts';

function stubClipboard(writeText: () => Promise<void>): void {
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  });
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  useToastsStore.getState().clear();
});

describe('copyText', () => {
  it('writes to the clipboard and says what it copied', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    stubClipboard(writeText);

    await copyText('the UID', 'uid-web');

    expect(writeText).toHaveBeenCalledWith('uid-web');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Copied the UID' }),
    ]);
  });

  it('says so when the clipboard refuses', async () => {
    stubClipboard(() => Promise.reject(new Error('denied')));

    await copyText('the UID', 'uid-web');

    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'error', message: 'Could not copy the UID' }),
    ]);
  });
});

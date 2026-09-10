import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SupportBundle from '../../src/components/SupportBundle';
import { useToastsStore } from '../../src/store/toasts';

function stub(ok: boolean) {
  const asked: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      asked.push(url);
      return Promise.resolve({
        ok,
        status: ok ? 200 : 403,
        blob: () => Promise.resolve(new Blob(['{}'], { type: 'application/json' })),
        json: () => Promise.resolve({ message: 'your role here is viewer; this needs admin' }),
      });
    }),
  );
  return asked;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.setState({ toasts: [], history: [] });
});

describe('SupportBundle', () => {
  it('asks the server for the bundle', async () => {
    const asked = stub(true);
    vi.stubGlobal('URL', {
      createObjectURL: () => 'blob:one',
      revokeObjectURL: () => undefined,
    });

    render(<SupportBundle />);
    await userEvent.click(screen.getByRole('button', { name: 'Save a support bundle' }));

    await waitFor(() => {
      expect(asked).toContain('/api/support');
    });
  });

  it('says what the server refused rather than a silent nothing', async () => {
    stub(false);

    render(<SupportBundle />);
    await userEvent.click(screen.getByRole('button', { name: 'Save a support bundle' }));

    await waitFor(() => {
      const toasts = useToastsStore.getState().toasts;
      expect(toasts.some((one) => one.message.includes('this needs admin'))).toBe(true);
    });
  });

  it('says what stopped the download rather than a silent nothing', async () => {
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('the connection went away'))),
    );

    render(<SupportBundle />);
    await userEvent.click(screen.getByRole('button', { name: 'Save a support bundle' }));

    await waitFor(() => {
      expect(
        useToastsStore.getState().toasts.some((one) => one.message === 'the connection went away'),
      ).toBe(true);
    });
    useToastsStore.getState().clear();
  });
});

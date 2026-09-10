import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SavedViewsList from '../../src/components/SavedViewsList';
import { useToastsStore } from '../../src/store/toasts';

function stub(views: unknown[], mayShare = true) {
  const calls: { url: string; method?: string }[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string, init?: RequestInit) => {
      calls.push({ url, method: init?.method });
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ views, mayShare }),
      });
    }),
  );
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('SavedViewsList', () => {
  it('says there are none rather than showing an empty list', async () => {
    stub([]);

    render(<SavedViewsList active />);

    expect(await screen.findByText(/None yet/)).toBeInTheDocument();
  });

  it('names each view, what it narrows to, and whose it is', async () => {
    stub([
      { id: 'a', name: 'crashing pods', view: 'resources', resource: 'pods', namespace: 'prod' },
      { id: 'b', name: 'posture', view: 'checks', shared: true },
    ]);

    render(<SavedViewsList active />);

    expect(await screen.findByText('crashing pods')).toBeInTheDocument();
    expect(screen.getByText('pods in prod')).toBeInTheDocument();
    expect(screen.getByText('yours')).toBeInTheDocument();
    expect(screen.getByText('everybody')).toBeInTheDocument();
  });

  it('forgets the one it was asked to', async () => {
    const calls = stub([{ id: 'a', name: 'crashing pods', view: 'resources' }]);

    render(<SavedViewsList active />);
    await userEvent.click(await screen.findByRole('button', { name: 'Forget' }));

    await waitFor(() => {
      const gone = calls.find((one) => one.method === 'DELETE');
      expect(gone?.url).toContain('id=a');
    });
  });

  it('will not let somebody forget a published view they cannot publish', async () => {
    stub([{ id: 'b', name: 'posture', view: 'checks', shared: true }], false);

    render(<SavedViewsList active />);

    expect(await screen.findByRole('button', { name: 'Forget' })).toBeDisabled();
  });

  it('says why it could not read them', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({}) })),
    );

    render(<SavedViewsList active />);

    expect(await screen.findByText(/saved views/i)).toBeInTheDocument();
  });

  it('says nothing is read while the dialog is closed', () => {
    const calls = stub([]);

    render(<SavedViewsList active={false} />);

    expect(calls).toHaveLength(0);
    expect(screen.getByText('reading them…')).toBeInTheDocument();
  });

  it('says what stopped it from forgetting one', async () => {
    useToastsStore.getState().clear();
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return Promise.reject(new Error('the store went away'));
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              views: [{ id: 'a', name: 'crashing pods', view: 'resources' }],
              mayShare: true,
            }),
        });
      }),
    );

    render(<SavedViewsList active />);
    await userEvent.click(await screen.findByRole('button', { name: 'Forget' }));

    await waitFor(() => {
      expect(
        useToastsStore.getState().toasts.some((one) => one.message === 'the store went away'),
      ).toBe(true);
    });
    useToastsStore.getState().clear();
  });
});

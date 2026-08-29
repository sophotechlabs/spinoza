import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import History from '../../src/components/History';
import type { HistoryEntry } from '../../src/lib/types';

function entry(extra: Partial<HistoryEntry> = {}): HistoryEntry {
  return {
    id: 1,
    at: '2026-08-29T09:30:00Z',
    verb: 'delete',
    name: 'web',
    outcome: 'done',
    ...extra,
  };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetcher = vi.fn((url: string, init?: RequestInit) => {
    void url;
    void init;
    return Promise.resolve({
      ok,
      status,
      json: () => Promise.resolve(body),
      text: () => Promise.resolve(JSON.stringify(body)),
    });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('History', () => {
  it('shows what spinoza did', async () => {
    stub({
      entries: [
        entry({
          verb: 'scale',
          kind: 'Deployment',
          resource: 'deployments',
          namespace: 'default',
          detail: 'to 3 replicas',
        }),
      ],
    });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('Deployment web')).toBeTruthy();
    expect(screen.getByText('scale')).toBeTruthy();
    expect(screen.getByText('default')).toBeTruthy();
    expect(screen.getByText('to 3 replicas')).toBeTruthy();
    expect(screen.getByText('Done')).toBeTruthy();
  });

  it('says so when nothing has been done yet', async () => {
    stub({ entries: [] });

    render(<History onOpen={vi.fn()} />);

    expect(
      await screen.findByText('Spinoza has not changed anything on this cluster yet.'),
    ).toBeTruthy();
  });

  it('says when it is showing only the newest page', async () => {
    stub({ entries: [entry()], more: true });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/showing the newest/)).toBeTruthy();
  });

  it('does not claim a page was trimmed when it was not', async () => {
    stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('web');
    expect(screen.queryByText(/showing the newest/)).toBeNull();
  });

  it('warns when spinoza is not recording', async () => {
    stub({ entries: [], reason: 'the history file is read-only' });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('the history file is read-only')).toBeTruthy();
  });

  it('opens the object a row names', async () => {
    stub({
      entries: [
        entry({ group: 'apps', version: 'v1', resource: 'deployments', kind: 'Deployment' }),
      ],
    });
    const onOpen = vi.fn();

    render(<History onOpen={onOpen} />);
    await userEvent.click(await screen.findByRole('button', { name: 'Deployment web' }));

    expect(onOpen).toHaveBeenCalledWith(
      expect.objectContaining({ resource: 'deployments', name: 'web' }),
    );
  });

  it('leaves a row with no resource unclickable', async () => {
    stub({ entries: [entry({ kind: 'Release' })] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('Release web');
    expect(screen.queryByRole('button', { name: 'Release web' })).toBeNull();
  });

  it('shows a spinner before the first answer', () => {
    stub({ entries: [] });

    render(<History onOpen={vi.fn()} />);

    expect(screen.getByRole('status')).toBeTruthy();
  });

  it('reports a first load that failed', async () => {
    stub({ message: 'the database went away' }, false, 500);

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('the database went away')).toBeTruthy();
  });

  it('falls back to its own words when the server explained nothing', async () => {
    stub({}, false, 500);

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/history request failed with status 500/)).toBeTruthy();
  });

  it('keeps the rows on screen and warns when a refresh fails', async () => {
    let at = -1;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        void url;
        at += 1;
        const failing = init?.method !== 'DELETE' && at > 0;
        return Promise.resolve({
          ok: !failing,
          status: failing ? 500 : 200,
          json: () =>
            Promise.resolve(
              failing ? { message: 'the database went away' } : { entries: [entry()] },
            ),
          text: () => Promise.resolve(''),
        });
      }),
    );

    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    expect(await screen.findByText(/the database went away/)).toBeTruthy();
    expect(screen.getByText('web')).toBeTruthy();
  });

  it('clears the history when asked', async () => {
    const fetcher = stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    await waitFor(() => {
      expect(fetcher.mock.calls.some((call) => call[1]?.method === 'DELETE')).toBe(true);
    });
  });

  it('says so when the clear itself was refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        void url;
        const clearing = init?.method === 'DELETE';
        return Promise.resolve({
          ok: !clearing,
          status: clearing ? 503 : 200,
          json: () =>
            Promise.resolve(
              clearing ? { message: 'spinoza is not recording' } : { entries: [entry()] },
            ),
          text: () => Promise.resolve(''),
        });
      }),
    );

    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Clear' }).hasAttribute('disabled')).toBe(false);
    });
    expect(screen.getByText('web')).toBeTruthy();
  });

  it('cannot be cleared when there is nothing to clear', async () => {
    stub({ entries: [] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('Spinoza has not changed anything on this cluster yet.');
    expect(screen.getByRole('button', { name: 'Clear' }).hasAttribute('disabled')).toBe(true);
  });
});

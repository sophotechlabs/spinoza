import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ViewSwitch from '../../src/components/ViewSwitch';
import { DESKTOP } from '../../src/lib/view';
import { useToastsStore } from '../../src/store/toasts';

interface Replies {
  view?: { window: boolean; hidden?: boolean };
  moved?: { switched: boolean; reason?: string };
  movedOk?: boolean;
  movedStatus?: number;
}

function stub(replies: Replies) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      calls.push(url);
      if (url.includes('/api/view/')) {
        return Promise.resolve({
          ok: replies.movedOk ?? true,
          status: replies.movedStatus ?? 200,
          json: () => Promise.resolve(replies.moved ?? { switched: true }),
        });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(replies.view ?? { window: false }),
      });
    }),
  );
  return calls;
}

function inWindow() {
  window.__SPINOZA_VIEW__ = DESKTOP;
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  delete window.__SPINOZA_VIEW__;
  useToastsStore.getState().clear();
});

describe('ViewSwitch', () => {
  it('offers nothing when this build has no window', async () => {
    stub({ view: { window: false } });

    const view = render(<ViewSwitch onLeft={vi.fn()} />);

    await waitFor(() => {
      expect(view.container).toBeEmptyDOMElement();
    });
  });

  it('offers the browser from inside the window', async () => {
    stub({ view: { window: true } });
    inWindow();

    render(<ViewSwitch onLeft={vi.fn()} />);

    expect(await screen.findByRole('button', { name: 'Browser' })).toBeInTheDocument();
  });

  it('offers the window from a browser tab', async () => {
    stub({ view: { window: true, hidden: true } });

    render(<ViewSwitch onLeft={vi.fn()} />);

    expect(await screen.findByRole('button', { name: 'Desktop' })).toBeInTheDocument();
  });

  it('moves to the browser', async () => {
    const user = userEvent.setup();
    const calls = stub({ view: { window: true }, moved: { switched: true } });
    inWindow();
    render(<ViewSwitch onLeft={vi.fn()} />);

    await user.click(await screen.findByRole('button', { name: 'Browser' }));

    await waitFor(() => {
      expect(calls.some((url) => url.includes('/api/view/browser'))).toBe(true);
    });
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('says so when the browser never came', async () => {
    const user = userEvent.setup();
    stub({
      view: { window: true },
      moved: { switched: false, reason: 'the browser did not open spinoza' },
    });
    inWindow();
    render(<ViewSwitch onLeft={vi.fn()} />);

    await user.click(await screen.findByRole('button', { name: 'Browser' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('did not open');
    });
    expect(useToastsStore.getState().toasts[0].tone).toBe('warn');
  });

  it('says so when the switch was refused outright', async () => {
    const user = userEvent.setup();
    stub({
      view: { window: true },
      movedOk: false,
      movedStatus: 500,
      moved: { switched: false, reason: '' },
    });
    inWindow();
    render(<ViewSwitch onLeft={vi.fn()} />);

    await user.click(await screen.findByRole('button', { name: 'Browser' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].tone).toBe('error');
    });
  });

  it('moves back to the window and tells the app', async () => {
    const user = userEvent.setup();
    const onLeft = vi.fn();
    const calls = stub({ view: { window: true, hidden: true }, moved: { switched: true } });
    render(<ViewSwitch onLeft={onLeft} />);

    await user.click(await screen.findByRole('button', { name: 'Desktop' }));

    await waitFor(() => {
      expect(onLeft).toHaveBeenCalledTimes(1);
    });
    expect(calls.some((url) => url.includes('/api/view/desktop'))).toBe(true);
  });

  it('reports a window that would not come back', async () => {
    const user = userEvent.setup();
    const onLeft = vi.fn();
    stub({ view: { window: true, hidden: true }, movedOk: false, movedStatus: 501 });
    render(<ViewSwitch onLeft={onLeft} />);

    await user.click(await screen.findByRole('button', { name: 'Desktop' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].tone).toBe('error');
    });
    expect(onLeft).not.toHaveBeenCalled();
  });

  it('carries on when the view lookup fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    );

    const view = render(<ViewSwitch onLeft={vi.fn()} />);

    await waitFor(() => {
      expect(view.container).toBeEmptyDOMElement();
    });
  });

  it('falls back to plain words when the failure carries none', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/view/')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.reject(new Error()),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ window: true, hidden: true }),
        });
      }),
    );
    render(<ViewSwitch onLeft={vi.fn()} />);

    await user.click(await screen.findByRole('button', { name: 'Desktop' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toBe('the switch did not happen');
    });
  });
});

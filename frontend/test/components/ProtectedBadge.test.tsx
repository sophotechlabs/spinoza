import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ProtectedBadge from '../../src/components/ProtectedBadge';
import type { ContextList, Protection } from '../../src/lib/types';
import { useContextsStore } from '../../src/store/contexts';
import { useToastsStore } from '../../src/store/toasts';

function list(protection: Protection): ContextList {
  return {
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection,
  };
}

function stubFetch(ok = true) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      calls.push(url);
      return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve({ ...list('open'), message: 'the file is read-only' }),
      });
    }),
  );
  return calls;
}

function show(protection: Protection) {
  useContextsStore.getState().setList(list(protection));
  return render(<ProtectedBadge />);
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('ProtectedBadge', () => {
  it('marks a protected cluster', () => {
    stubFetch();

    show('protected');

    expect(screen.getByRole('button', { name: 'protected' })).toBeInTheDocument();
  });

  it('stays out of the way on an open cluster', () => {
    stubFetch();

    const view = show('open');

    expect(view.container).toBeEmptyDOMElement();
  });

  it('lifts the protection when it is clicked', async () => {
    const user = userEvent.setup();
    const calls = stubFetch();

    show('protected');
    await user.click(screen.getByRole('button', { name: 'protected' }));

    await waitFor(() => {
      expect(useContextsStore.getState().list.protection).toBe('open');
    });
    expect(calls[0]).toContain('protected=false');
    expect(useToastsStore.getState().toasts[0].message).toBe('This cluster is no longer protected');
  });

  it('reports a change the backend refused', async () => {
    const user = userEvent.setup();
    stubFetch(false);

    show('protected');
    await user.click(screen.getByRole('button', { name: 'protected' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('read-only');
    });
    expect(useContextsStore.getState().list.protection).toBe('protected');
  });

  it('falls back to plain words when the failure carries none', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.reject(new Error()) })),
    );

    show('protected');
    await user.click(screen.getByRole('button', { name: 'protected' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toBe('the answer could not be saved');
    });
  });
});

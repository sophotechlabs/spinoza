import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ProtectionPrompt from '../../src/components/ProtectionPrompt';
import type { ContextList, Protection } from '../../src/lib/types';
import { useContextsStore } from '../../src/store/contexts';
import { useToastsStore } from '../../src/store/toasts';

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});
const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

function list(protection: Protection, name = 'p-mk1'): ContextList {
  return {
    current: { kubeconfig: '', name },
    kubeconfigs: [],
    protection,
  };
}

function stubFetch(protection: Protection, ok = true) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      calls.push(url);
      return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve({ ...list(protection), message: 'the file is read-only' }),
      });
    }),
  );
  return calls;
}

function show(protection: Protection, name = 'p-mk1') {
  useContextsStore.getState().setList(list(protection, name));
  return render(<ProtectionPrompt />);
}

beforeEach(() => {
  useToastsStore.getState().clear();
  showModal.mockClear();
  close.mockClear();
  HTMLDialogElement.prototype.showModal = showModal;
  HTMLDialogElement.prototype.close = close;
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('ProtectionPrompt', () => {
  it('asks once about a cluster spinoza has not seen', () => {
    stubFetch('protected');

    show('unknown');

    expect(showModal).toHaveBeenCalled();
    expect(screen.getByText(/is new here/)).toBeInTheDocument();
  });

  it('says nothing about a cluster that already has an answer', () => {
    stubFetch('protected');

    const view = show('open');

    expect(view.container).toBeEmptyDOMElement();
  });

  it('says nothing while no context is connected', () => {
    stubFetch('protected');

    const view = show('unknown', '');

    expect(view.container).toBeEmptyDOMElement();
  });

  it('protects the cluster and says so', async () => {
    const user = userEvent.setup();
    const calls = stubFetch('protected');

    show('unknown');
    await user.click(screen.getByRole('button', { name: 'Protect it' }));

    await waitFor(() => {
      expect(useContextsStore.getState().list.protection).toBe('protected');
    });
    expect(calls[0]).toContain('protected=true');
    expect(useToastsStore.getState().toasts[0].message).toBe('p-mk1 is protected');
  });

  it('leaves the cluster open without a word', async () => {
    const user = userEvent.setup();
    const calls = stubFetch('open');

    show('unknown');
    await user.click(screen.getByRole('button', { name: 'Leave it open' }));

    await waitFor(() => {
      expect(useContextsStore.getState().list.protection).toBe('open');
    });
    expect(calls[0]).toContain('protected=false');
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('goes away once the question is answered', async () => {
    const user = userEvent.setup();
    stubFetch('open');

    const view = show('unknown');
    await user.click(screen.getByRole('button', { name: 'Leave it open' }));

    await waitFor(() => {
      expect(view.container).toBeEmptyDOMElement();
    });
  });

  it('reports an answer the backend refused', async () => {
    const user = userEvent.setup();
    stubFetch('open', false);

    show('unknown');
    await user.click(screen.getByRole('button', { name: 'Protect it' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toContain('read-only');
    });
    expect(useContextsStore.getState().list.protection).toBe('unknown');
  });

  it('falls back to plain words when the failure carries none', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.reject(new Error()) })),
    );

    show('unknown');
    await user.click(screen.getByRole('button', { name: 'Protect it' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toBe('the answer could not be saved');
    });
  });
});

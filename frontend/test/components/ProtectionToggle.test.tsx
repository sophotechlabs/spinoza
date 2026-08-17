import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ProtectionToggle from '../../src/components/ProtectionToggle';
import type { ContextList, Protection } from '../../src/lib/types';
import { useContextsStore } from '../../src/store/contexts';
import { useToastsStore } from '../../src/store/toasts';

function list(protection: Protection, name = 'p-mk1'): ContextList {
  return {
    current: { kubeconfig: '', name },
    kubeconfigs: [],
    protection,
  };
}

function stubFetch(answer: Protection, ok = true) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      calls.push(url);
      return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve({ ...list(answer), message: 'the file is read-only' }),
      });
    }),
  );
  return calls;
}

function show(protection: Protection, name = 'p-mk1') {
  useContextsStore.getState().setList(list(protection, name));
  return render(<ProtectionToggle />);
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('ProtectionToggle', () => {
  it('marks a protected cluster', () => {
    stubFetch('open');

    show('protected');

    expect(screen.getByRole('button', { name: 'Protected cluster' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('says an open cluster is open', () => {
    stubFetch('protected');

    show('open');

    expect(screen.getByRole('button', { name: 'Open cluster' })).toHaveAttribute(
      'aria-pressed',
      'false',
    );
  });

  it('leaves the question to the prompt while the cluster is new', () => {
    stubFetch('protected');

    const view = show('unknown');

    expect(view.container).toBeEmptyDOMElement();
  });

  it('stays out of the way with no context connected', () => {
    stubFetch('protected');

    const view = show('open', '');

    expect(view.container).toBeEmptyDOMElement();
  });

  it('lifts the protection', async () => {
    const user = userEvent.setup();
    const calls = stubFetch('open');

    show('protected');
    await user.click(screen.getByRole('button', { name: 'Protected cluster' }));

    await waitFor(() => {
      expect(useContextsStore.getState().list.protection).toBe('open');
    });
    expect(calls[0]).toContain('protected=false');
    expect(useToastsStore.getState().toasts[0].message).toBe('p-mk1 is open again');
  });

  it('protects the cluster again', async () => {
    const user = userEvent.setup();
    const calls = stubFetch('protected');

    show('open');
    await user.click(screen.getByRole('button', { name: 'Open cluster' }));

    await waitFor(() => {
      expect(useContextsStore.getState().list.protection).toBe('protected');
    });
    expect(calls[0]).toContain('protected=true');
    expect(useToastsStore.getState().toasts[0].message).toBe('p-mk1 is protected');
  });

  it('reports a change the backend refused', async () => {
    const user = userEvent.setup();
    stubFetch('open', false);

    show('protected');
    await user.click(screen.getByRole('button', { name: 'Protected cluster' }));

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
    await user.click(screen.getByRole('button', { name: 'Protected cluster' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts[0].message).toBe('the answer could not be saved');
    });
  });

  it('shows a lock rather than a word', () => {
    stubFetch('open');

    show('protected');

    const button = screen.getByRole('button', { name: 'Protected cluster' });
    expect(button.textContent).toBe('');
    expect(button.querySelector('svg')).not.toBeNull();
    expect(button).toHaveAttribute('title', expect.stringContaining('Click to lift'));
  });

  it('shows an open lock on an open cluster', () => {
    stubFetch('protected');

    show('open');

    const button = screen.getByRole('button', { name: 'Open cluster' });
    expect(button.querySelector('svg')).not.toBeNull();
    expect(button).toHaveAttribute('title', expect.stringContaining('Click to protect'));
  });
});

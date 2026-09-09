import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, waitFor } from '@testing-library/react';
import ProtectionOffer from '../../src/components/ProtectionOffer';
import type { ContextList, Protection } from '../../src/lib/types';
import { bumpClusterEpoch } from '../../src/store/cluster';
import { useContextsStore } from '../../src/store/contexts';
import { useToastsStore } from '../../src/store/toasts';

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
  act(() => {
    useContextsStore.getState().setList(list(protection, name));
  });
  return render(<ProtectionOffer />);
}

function offer() {
  return useToastsStore.getState().toasts[0];
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    useToastsStore.getState().clear();
  });
});

describe('offering to protect a cluster spinoza has not seen', () => {
  it('asks in a toast rather than a dialog that takes over the screen', () => {
    show('unknown', 'p-mk2');

    expect(document.querySelector('dialog')).toBeNull();
    expect(offer().message).toContain('p-mk2 is new here');
    expect(offer().actions?.map((one) => one.label)).toEqual(['Protect', 'Leave unprotected']);
  });

  it('says nothing about a cluster whose answer is already known', () => {
    show('open');

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('says nothing when no context is current', () => {
    show('unknown', '');

    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('protects the cluster and says so', async () => {
    const calls = stubFetch('protected');
    show('unknown', 'p-mk2');

    act(() => {
      offer().actions?.[0].run();
    });

    await waitFor(() => {
      expect(calls).toEqual(['/api/protection?protected=true']);
    });
    await waitFor(() => {
      expect(useToastsStore.getState().toasts.at(-1)?.message).toBe('p-mk2 is protected');
    });
  });

  it('records the answer without congratulating itself when left unprotected', async () => {
    const calls = stubFetch('open');
    show('unknown', 'p-mk2');

    act(() => {
      offer().actions?.[1].run();
    });

    await waitFor(() => {
      expect(calls).toEqual(['/api/protection?protected=false']);
    });
    expect(
      useToastsStore.getState().toasts.some((one) => one.message.includes('is protected')),
    ).toBe(false);
  });

  it('says why the answer could not be saved', async () => {
    stubFetch('unknown', false);
    show('unknown', 'p-mk2');

    act(() => {
      offer().actions?.[0].run();
    });

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.at(-1)?.tone).toBe('error');
    });
    expect(useToastsStore.getState().toasts.at(-1)?.message).toBe('the file is read-only');
  });

  it('says something even when the failure carried no words', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error(''))),
    );
    show('unknown', 'p-mk2');

    act(() => {
      offer().actions?.[0].run();
    });

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.at(-1)?.message).toBe(
        'the answer could not be saved',
      );
    });
  });

  it('does not ask twice about one cluster when the component re-renders', () => {
    const view = show('unknown', 'p-mk2');

    view.rerender(<ProtectionOffer />);
    act(() => {
      useContextsStore.getState().setList(list('unknown', 'p-mk2'));
    });

    expect(useToastsStore.getState().toasts).toHaveLength(1);
  });

  it('asks once for a cluster, not on every refresh of the list', () => {
    show('unknown', 'p-mk2');

    act(() => {
      useContextsStore.getState().setList(list('unknown', 'p-mk2'));
    });

    expect(useToastsStore.getState().toasts).toHaveLength(1);
  });

  it('does not ask again when the same cluster is asked about twice', () => {
    show('unknown', 'p-mk2');

    act(() => {
      useContextsStore.getState().setList(list('open', 'p-mk2'));
    });
    act(() => {
      useContextsStore.getState().setList(list('unknown', 'p-mk2'));
    });

    expect(useToastsStore.getState().toasts).toHaveLength(1);
  });

  it('asks again once the window moves to another cluster', () => {
    show('unknown', 'p-mk2');

    act(() => {
      bumpClusterEpoch();
      useContextsStore.getState().setList(list('unknown', 'p-mk1'));
    });

    expect(useToastsStore.getState().toasts).toHaveLength(2);
  });
});

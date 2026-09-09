import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ClusterStrip from '../../src/components/ClusterStrip';
import { adoptClusters, useClustersStore } from '../../src/store/clusters';
import { useContextsStore } from '../../src/store/contexts';
import { MK1, listOf } from '../helpers-clusters';

function contexts() {
  return {
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [
      {
        label: '/home/arch/.kube/config',
        path: '',
        removable: false,
        contexts: [
          { name: 'p-mk1', cluster: 'p-mk1' },
          { name: 'p-mk2', cluster: 'p-mk2' },
        ],
      },
    ],
    protection: 'open' as const,
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
  useClustersStore.getState().reset();
  useContextsStore.getState().reset();
});

describe('the strip carries the way to open a cluster', () => {
  it('mounts the real picker, not a placeholder', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(contexts()) })),
    );
    act(() => {
      adoptClusters(listOf(MK1));
    });

    render(<ClusterStrip onShown={vi.fn()} />);

    expect(await screen.findByLabelText('Open a cluster')).toBeInTheDocument();
  });

  it('lists the contexts when the plus is used', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(contexts()) })),
    );
    act(() => {
      adoptClusters(listOf(MK1));
    });
    render(<ClusterStrip onShown={vi.fn()} />);

    const plus = await screen.findByLabelText('Open a cluster');
    await user.click(plus);

    const menu = plus.parentElement;
    if (menu === null) {
      throw new Error('the plus has no menu around it');
    }
    expect(within(menu).getByRole('button', { name: 'p-mk2' })).toBeVisible();
    expect(within(menu).getByRole('button', { name: 'Manage kubeconfigs' })).toBeVisible();
  });

  it('keeps the menu out of the strip that scrolls sideways', () => {
    act(() => {
      adoptClusters(listOf(MK1));
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(contexts()) })),
    );

    render(<ClusterStrip onShown={vi.fn()} />);

    const tabs = screen.getByRole('navigation', { name: 'Open clusters' });
    expect(tabs.className).toContain('overflow-x-auto');
    expect(tabs.querySelector('summary')).toBeNull();
  });
});

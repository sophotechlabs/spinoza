import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category, TrafficSupport, View } from '../../src/lib/types';
import Sidebar from '../../src/components/Sidebar';
import { clearCatalog } from '../../src/store/catalog';
import { useTrafficStore } from '../../src/store/traffic';
import { makeCategory, makeDescriptor, rejectsWith } from '../helpers';

const categories: Category[] = [
  makeCategory('Workloads', [
    makeDescriptor({ group: '', version: 'v1', resource: 'pods', kind: 'Pod' }),
  ]),
];

function stubFetch(support: TrafficSupport | Error): void {
  const fetchMock = vi.fn().mockImplementation((url: string) => {
    if (url.startsWith('/api/traffic/support')) {
      if (support instanceof Error) {
        return Promise.reject(support);
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(support) });
    }
    if (url.startsWith('/api/resources/counts')) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ counts: {} }) });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
  });
  vi.stubGlobal('fetch', fetchMock);
}

function renderSidebar(view: View = 'resources', onSelectView = vi.fn()) {
  render(
    <Sidebar view={view} activeResource={null} onSelect={vi.fn()} onSelectView={onSelectView} />,
  );
  return onSelectView;
}

afterEach(() => {
  vi.unstubAllGlobals();
  clearCatalog();
  act(() => {
    useTrafficStore.getState().remember({ available: false });
  });
});

describe('the Traffic entry', () => {
  it('is disabled and says what to configure when the mesh has no workload labels', async () => {
    stubFetch({ available: false, reason: 'Add flow:labelsContext to the Cilium values' });
    renderSidebar();

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', 'Add flow:labelsContext to the Cilium values');
  });

  it('falls back to a not-found title when no reason came back', async () => {
    stubFetch({ available: false });
    renderSidebar();

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toHaveAttribute('title', 'Traffic is not found in this cluster');
  });

  it('reports a failed probe as the title', async () => {
    stubFetch(new Error('traffic support request failed with status 503'));
    renderSidebar();

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toHaveAttribute('title', 'traffic support request failed with status 503');
  });

  it('reports a non-error rejection with a plain message', async () => {
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/traffic/support')) {
        return rejectsWith('boom')();
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderSidebar();

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toHaveAttribute('title', 'the traffic probe failed');
  });

  it('says it is still checking before the first answer arrives', async () => {
    let answer: () => void;
    const held = new Promise((resolve) => {
      answer = () => {
        resolve({
          ok: true,
          json: () => Promise.resolve({ available: true, source: 'Cilium Hubble' }),
        });
      };
    });
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/traffic/support')) {
        return held;
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderSidebar();

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute(
      'title',
      'checking whether a service mesh is exporting flow metrics',
    );

    await act(async () => {
      answer();
      await held;
    });
    expect(screen.getByRole('button', { name: 'Traffic' })).toBeEnabled();
  });

  it('keeps looking after a failed probe', async () => {
    vi.useFakeTimers();
    let call = 0;
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/traffic/support')) {
        call += 1;
        if (call === 1) {
          return rejectsWith(new Error('traffic support request failed with status 503'))();
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ available: true, source: 'Cilium Hubble' }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderSidebar();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByRole('button', { name: 'Traffic' })).toBeDisabled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(screen.getByRole('button', { name: 'Traffic' })).toBeEnabled();
    vi.useRealTimers();
  });

  it('opens the view and names the mesh once the labels are there', async () => {
    stubFetch({ available: true, source: 'Cilium Hubble' });
    const onSelectView = renderSidebar();

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toBeEnabled();
    expect(button).toHaveAttribute('title', 'Cilium Hubble');

    await userEvent.click(button);
    expect(onSelectView).toHaveBeenCalledWith('traffic');
  });

  it('marks itself as the current view when traffic is open', async () => {
    stubFetch({ available: true, source: 'Cilium Hubble' });
    renderSidebar('traffic');

    const button = await screen.findByRole('button', { name: 'Traffic' });
    expect(button).toHaveAttribute('aria-current', 'page');
  });
});

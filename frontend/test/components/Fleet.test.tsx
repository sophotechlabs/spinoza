import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Fleet from '../../src/components/Fleet';
import { useClustersStore } from '../../src/store/clusters';
import { MK1, MK2, showing } from '../helpers-clusters';

function nodes(total: number, ready: number) {
  return {
    total,
    ready,
    unschedulable: 0,
    cpuAllocatableMilli: 0,
    cpuUsedMilli: 0,
    memAllocatableMi: 0,
    memUsedMi: 0,
    usageKnown: false,
  };
}

function pods(total: number, running: number, known = true) {
  return { total, running, pending: 0, failed: 0, succeeded: 0, known, capped: [] };
}

function line(cluster: string, context: string, version: string) {
  return { cluster, context, version, nodes: nodes(3, 3), pods: pods(40, 39), warnings: 0 };
}

function stub(bodies: Record<string, unknown>) {
  const fetcher = vi.fn((url: string) => {
    const key = Object.keys(bodies).find((one) => url.includes(one)) ?? '';
    return Promise.resolve({
      ok: true,
      status: 200,
      json: () => Promise.resolve(bodies[key] ?? {}),
    });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useClustersStore.getState().reset();
});

describe('the fleet view', () => {
  it('shows a line for every cluster and one total', async () => {
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': {
        clusters: [line(MK1, 'p-mk1', 'v1.34.1'), line(MK2, 'p-mk2', 'v1.33.0')],
        nodes: nodes(6, 6),
        pods: pods(60, 57),
      },
    });

    render(<Fleet onPick={vi.fn()} />);

    expect(await screen.findByText('v1.34.1')).toBeTruthy();
    expect(screen.getByText('v1.33.0')).toBeTruthy();
    expect(screen.getByText('Everything open')).toBeTruthy();
    expect(screen.getByText('60/57'.replace('60/57', '57/60'))).toBeTruthy();
  });

  it('goes to a cluster when its line is clicked', async () => {
    const user = userEvent.setup();
    const onPick = vi.fn();
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': {
        clusters: [line(MK1, 'p-mk1', 'v1.34.1')],
        nodes: nodes(3, 3),
        pods: pods(40, 39),
      },
    });
    render(<Fleet onPick={onPick} />);

    await user.click(await screen.findByRole('button', { name: /p-mk1/ }));

    expect(onPick).toHaveBeenCalledWith(MK1);
  });

  it('says why a cluster could not be surveyed', async () => {
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': {
        clusters: [{ ...line(MK2, 'p-mk2', ''), reason: 'the apiserver refused' }],
        nodes: nodes(0, 0),
        pods: pods(0, 0, false),
        error: 'p-mk2: the apiserver refused',
      },
    });

    render(<Fleet onPick={vi.fn()} />);

    expect(await screen.findByText('p-mk2: the apiserver refused')).toBeTruthy();
  });

  it('counts what is on the clusters', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': { clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) },
      '/api/resources/fleet': {
        kinds: [{ key: '/v1/pods', total: 60, failing: 2, perCluster: { [MK1]: 40, [MK2]: 20 } }],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'What is on them' }));

    expect(await screen.findByText('pods')).toBeTruthy();
    expect(screen.getByText('2 unwell')).toBeTruthy();
  });

  it('lists the images and the drift between them', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': { clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) },
      '/api/images/fleet': {
        images: [
          {
            image: 'nginx:1.27',
            repo: 'nginx',
            tag: '1.27',
            pods: 3,
            clusters: [MK1],
            skew: ['1.25', '1.27'],
          },
        ],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Images' }));

    expect(await screen.findByText('nginx:1.27')).toBeTruthy();
    expect(screen.getByText('nginx is at 1.25 · 1.27')).toBeTruthy();
  });

  it('says so when what is on the clusters cannot be read', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/resources/fleet')) {
          return Promise.reject(new Error('the inventory went away'));
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) }),
        });
      }),
    );
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'What is on them' }));

    expect(await screen.findByText(/the inventory went away/)).toBeTruthy();
  });

  it('says so when the images cannot be read', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/images/fleet')) {
          return Promise.reject(new Error('the images went away'));
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) }),
        });
      }),
    );
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Images' }));

    expect(await screen.findByText(/the images went away/)).toBeTruthy();
  });

  it('lists the releases and the drift between them', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': { clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) },
      '/api/helm/fleet': {
        releases: [
          {
            cluster: MK1,
            name: 'loki',
            namespace: 'monitoring',
            chart: 'loki',
            chartVersion: '6.1.0',
            appVersion: '3',
            revision: 1,
            status: 'deployed',
            updated: '',
            skew: '6.1.0 · 6.2.0',
          },
        ],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Releases' }));

    expect(await screen.findByText('loki')).toBeTruthy();
    expect(screen.getByText('loki is at 6.1.0 · 6.2.0')).toBeTruthy();
  });

  it('says how far a delivered app spreads', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': { clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) },
      '/api/gitops/fleet': {
        apps: [
          {
            cluster: MK1,
            engine: 'flux',
            kind: 'Kustomization',
            group: '',
            version: 'v1',
            resource: 'kustomizations',
            name: 'platform',
            namespace: 'flux-system',
            ready: 'True',
            spread: 2,
          },
        ],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Delivery' }));

    expect(await screen.findByText('platform')).toBeTruthy();
    expect(screen.getByText('everywhere')).toBeTruthy();
  });

  it('says so when the releases cannot be read', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/helm/fleet')) {
          return Promise.reject(new Error('helm went away'));
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) }),
        });
      }),
    );
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Releases' }));

    expect(await screen.findByText(/helm went away/)).toBeTruthy();
  });

  it('says so when the delivery cannot be read', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/gitops/fleet')) {
          return Promise.reject(new Error('delivery went away'));
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) }),
        });
      }),
    );
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Delivery' }));

    expect(await screen.findByText(/delivery went away/)).toBeTruthy();
  });

  it('says so when the fleet cannot be read at all', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));

    render(<Fleet onPick={vi.fn()} />);

    expect(await screen.findByText(/network down/)).toBeTruthy();
  });
});

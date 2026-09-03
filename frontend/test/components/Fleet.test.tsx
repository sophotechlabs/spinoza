import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, within } from '@testing-library/react';
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
        clusters: [
          { ...line(MK1, 'p-mk1', 'v1.34.1'), warnings: 2 },
          { ...line(MK2, 'p-mk2', 'v1.33.0'), warnings: 3 },
        ],
        nodes: nodes(6, 6),
        pods: pods(60, 57),
      },
    });

    render(<Fleet onPick={vi.fn()} />);

    expect(await screen.findByText('v1.34.1')).toBeTruthy();
    expect(screen.getByText('v1.33.0')).toBeTruthy();
    const total = screen.getByRole('row', { name: /Everything open/ });
    const cells = within(total)
      .getAllByRole('cell')
      .map((cell) => cell.textContent);
    expect(cells).toEqual(['Everything open', '—', '6/6', '57/60', '5', '—']);
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

  it('adds API identity only when inventory resource names collide', async () => {
    const user = userEvent.setup();
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': { clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) },
      '/api/resources/fleet': {
        kinds: [
          { key: '/v1/events', total: 4, perCluster: { [MK1]: 4 } },
          { key: 'events.k8s.io/v1/events', total: 6, perCluster: { [MK1]: 6 } },
          { key: '/v1/pods', total: 2, perCluster: { [MK1]: 2 } },
        ],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'What is on them' }));

    expect(await screen.findByText('core/v1', { exact: false })).toBeInTheDocument();
    expect(screen.getByText('events.k8s.io/v1', { exact: false })).toBeInTheDocument();
    const podsLabel = screen.getByText('pods').parentElement;
    expect(podsLabel).toHaveTextContent(/^pods$/);
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

  it('says when the image list stopped at its cap', async () => {
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
          },
        ],
        total: 1200,
        truncated: true,
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Images' }));

    expect(await screen.findByText('Showing 1 of 1200 images.')).toBeTruthy();
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

  it('shows the kinds of same-name delivery objects', async () => {
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
            group: 'kustomize.toolkit.fluxcd.io',
            version: 'v1',
            resource: 'kustomizations',
            name: 'podinfo',
            namespace: 'flux-system',
            ready: 'True',
          },
          {
            cluster: MK2,
            engine: 'argo',
            kind: 'Application',
            group: 'argoproj.io',
            version: 'v1alpha1',
            resource: 'applications',
            name: 'podinfo',
            namespace: 'argocd',
            ready: 'True',
          },
        ],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Delivery' }));

    expect(await screen.findByText('Kustomization', { exact: false })).toBeInTheDocument();
    expect(screen.getByText('Application', { exact: false })).toBeInTheDocument();
    expect(screen.getAllByText('podinfo')).toHaveLength(2);
  });

  it('adds API identity only after name, Kind, namespace, and cluster still collide', async () => {
    const user = userEvent.setup();
    const base = {
      cluster: MK1,
      engine: 'flux',
      kind: 'Kustomization',
      group: 'kustomize.toolkit.fluxcd.io',
      version: 'v1',
      resource: 'kustomizations',
      name: 'checkout',
      namespace: 'flux-system',
      ready: 'True',
    };
    act(() => {
      showing(MK1);
    });
    stub({
      '/api/overview/fleet': { clusters: [], nodes: nodes(0, 0), pods: pods(0, 0, false) },
      '/api/gitops/fleet': {
        apps: [
          base,
          { ...base, name: 'different' },
          {
            ...base,
            kind: 'HelmRelease',
            group: 'helm.toolkit.fluxcd.io',
            resource: 'helmreleases',
          },
          { ...base, namespace: 'staging' },
          { ...base, cluster: MK2 },
          {
            ...base,
            group: 'argoproj.io',
            version: 'v1alpha1',
            resource: 'applications',
          },
          { ...base, name: 'billing' },
          { ...base, name: 'billing', version: 'v1beta1' },
        ],
      },
    });
    render(<Fleet onPick={vi.fn()} />);
    await screen.findByText('Everything open');

    await user.click(screen.getByRole('button', { name: 'Delivery' }));

    expect(
      await screen.findAllByText('· Kustomization · kustomize.toolkit.fluxcd.io/v1'),
    ).toHaveLength(2);
    expect(screen.getByText('· Kustomization · argoproj.io/v1alpha1')).toBeInTheDocument();
    expect(
      screen.getByText('· Kustomization · kustomize.toolkit.fluxcd.io/v1beta1'),
    ).toBeInTheDocument();
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

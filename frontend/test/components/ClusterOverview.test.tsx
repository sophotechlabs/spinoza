import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import type { ClusterOverview as Overview } from '../../src/lib/types';
import ClusterOverview from '../../src/components/ClusterOverview';

function overview(patch: Partial<Overview> = {}): Overview {
  return {
    version: 'v1.36.1',
    nodes: {
      total: 3,
      ready: 3,
      unschedulable: 0,
      cpuAllocatableMilli: 12000,
      cpuUsedMilli: 3000,
      memAllocatableMi: 32768,
      memUsedMi: 8192,
      usageKnown: true,
    },
    pods: { total: 40, running: 38, pending: 1, failed: 1, succeeded: 0, known: true },
    warnings: [],
    ...patch,
  };
}

function stub(data: Overview): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(data) }),
  );
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('ClusterOverview', () => {
  it('waits for the first answer', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    );
    render(<ClusterOverview />);

    expect(screen.getByText('Loading the cluster overview')).toBeInTheDocument();
  });

  it('names the kubernetes version and the node tally', async () => {
    stub(overview());
    render(<ClusterOverview />);

    expect(await screen.findByText('v1.36.1')).toBeInTheDocument();
    expect(screen.getByText('3 ready')).toBeInTheDocument();
  });

  it('says a version it never learned is unknown', async () => {
    stub(overview({ version: '' }));
    render(<ClusterOverview />);

    expect(await screen.findByText('unknown')).toBeInTheDocument();
  });

  it('counts cordoned and not-ready nodes apart', async () => {
    stub(
      overview({
        nodes: {
          total: 5,
          ready: 3,
          unschedulable: 1,
          cpuAllocatableMilli: 0,
          cpuUsedMilli: 0,
          memAllocatableMi: 0,
          memUsedMi: 0,
          usageKnown: false,
        },
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('3 ready, 1 cordoned, 2 not ready')).toBeInTheDocument();
    expect(screen.getAllByText('- / -')).toHaveLength(2);
  });

  it('breaks the pod tally down by phase', async () => {
    stub(overview());
    render(<ClusterOverview />);

    expect(
      await screen.findByText('38 running, 1 pending, 1 failed, 0 succeeded'),
    ).toBeInTheDocument();
    expect(screen.getByText('40')).toBeInTheDocument();
  });

  it('says so when the pod tally could not be taken', async () => {
    stub(
      overview({
        pods: { total: 0, running: 0, pending: 0, failed: 0, succeeded: 0, known: false },
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('the tally could not be taken')).toBeInTheDocument();
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
  });

  it('shows usage against allocatable capacity', async () => {
    stub(overview());
    render(<ClusterOverview />);

    expect(await screen.findByText('3000m / 12000m')).toBeInTheDocument();
    expect(screen.getByText('8.0Gi / 32.0Gi')).toBeInTheDocument();
    expect(screen.getAllByText('25%')).toHaveLength(2);
  });

  it('explains a missing metrics-server instead of drawing a zero bar', async () => {
    stub(
      overview({
        nodes: {
          total: 1,
          ready: 1,
          unschedulable: 0,
          cpuAllocatableMilli: 4000,
          cpuUsedMilli: 0,
          memAllocatableMi: 8192,
          memUsedMi: 0,
          usageKnown: false,
        },
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText(/Live usage needs metrics-server/)).toBeInTheDocument();
    expect(screen.getByText('- / 4000m')).toBeInTheDocument();
    expect(screen.getByText('- / 8.0Gi')).toBeInTheDocument();
  });

  it('shows a zero rather than a blank when nothing is in use yet', async () => {
    stub(
      overview({
        nodes: {
          total: 1,
          ready: 1,
          unschedulable: 0,
          cpuAllocatableMilli: 4000,
          cpuUsedMilli: 0,
          memAllocatableMi: 8192,
          memUsedMi: 0,
          usageKnown: true,
        },
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('0 / 4000m')).toBeInTheDocument();
    expect(screen.getByText('0 / 8.0Gi')).toBeInTheDocument();
  });

  it('lists the warnings it was given', async () => {
    stub(
      overview({
        warnings: [
          {
            namespace: 'flux-system',
            object: 'Pod/web-1',
            reason: 'BackOff',
            message: 'back-off restarting failed container',
            count: 4,
            lastSeen: new Date(Date.now() - 90000).toISOString(),
          },
        ],
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('Pod/web-1')).toBeInTheDocument();
    expect(screen.getByText('BackOff')).toBeInTheDocument();
    expect(screen.getByText('back-off restarting failed container')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByText('1m')).toBeInTheDocument();
  });

  it('says when there is nothing to worry about', async () => {
    stub(overview());
    render(<ClusterOverview />);

    expect(
      await screen.findByText('No warning events in the cluster right now.'),
    ).toBeInTheDocument();
  });

  it('carries a partial-data warning from the server', async () => {
    stub(overview({ error: '1 of 3 resource types could not be listed: nodes (forbidden)' }));
    render(<ClusterOverview />);

    expect(await screen.findByText(/nodes \(forbidden\)/)).toBeInTheDocument();
    expect(screen.getByText('Partial data.')).toBeInTheDocument();
  });

  it('shows the failure when nothing loaded at all', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('overview is down')));
    render(<ClusterOverview />);

    expect(await screen.findByText('overview is down')).toBeInTheDocument();
    expect(screen.getByText('The cluster overview could not be loaded')).toBeInTheDocument();
  });

  it('keeps the last good answer and offers a retry when a later poll fails', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(overview()) })
      .mockRejectedValue(new Error('overview is down'));
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers();
    render(<ClusterOverview />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('v1.36.1')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(screen.getByText('v1.36.1')).toBeInTheDocument();
    expect(screen.getByText('The cluster overview stopped updating.')).toBeInTheDocument();
    const before = fetchMock.mock.calls.length;
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
  });
});

describe('the gitops controllers card', () => {
  it('rolls up both ecosystems', async () => {
    stub(
      overview({
        controllers: [
          {
            controller: 'argocd',
            name: 'argocd-application-controller',
            namespace: 'argocd',
            ready: 1,
            wanted: 1,
          },
          {
            controller: 'flux',
            name: 'source-controller',
            namespace: 'flux-system',
            ready: 1,
            wanted: 1,
          },
        ],
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('GitOps controllers')).toBeInTheDocument();
    expect(screen.getByText('argocd-application-controller')).toBeInTheDocument();
    expect(screen.getByText('Argo CD')).toBeInTheDocument();
    expect(screen.getByText('Flux')).toBeInTheDocument();
  });

  it('says how many of each are running', async () => {
    stub(
      overview({
        controllers: [
          { controller: 'argocd', name: 'argocd-server', namespace: 'argocd', ready: 0, wanted: 2 },
        ],
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('0 of 2')).toBeInTheDocument();
  });

  it('names a controller it does not know', async () => {
    stub(
      overview({
        controllers: [{ controller: 'other', name: 'thing', namespace: 'x', ready: 1, wanted: 1 }],
      }),
    );
    render(<ClusterOverview />);

    expect(await screen.findByText('other')).toBeInTheDocument();
  });

  it('stays away from a cluster with no gitops', async () => {
    stub(overview());
    render(<ClusterOverview />);

    await screen.findByText('Cluster');
    expect(screen.queryByText('GitOps controllers')).not.toBeInTheDocument();
  });
});

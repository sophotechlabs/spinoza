import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { HelmRelease, HelmReleases as Releases } from '../../src/lib/types';
import HelmReleases from '../../src/components/HelmReleases';

vi.mock('../../src/components/HelmReleaseDetail', () => ({
  default: ({ release, onClose }: { release: HelmRelease; onClose: () => void }) => (
    <div data-testid="release-detail">
      detail for {release.name}
      <button type="button" onClick={onClose}>
        close-detail
      </button>
    </div>
  ),
}));

function release(patch: Partial<HelmRelease> = {}): HelmRelease {
  return {
    name: 'podinfo',
    namespace: 'demo',
    chart: 'podinfo',
    chartVersion: '6.9.2',
    appVersion: '6.9.2',
    revision: 3,
    status: 'deployed',
    updated: new Date(Date.now() - 3600000).toISOString(),
    description: 'Upgrade complete',
    ...patch,
  };
}

function stub(data: Releases): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(data) }),
  );
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('HelmReleases', () => {
  it('waits for the first answer', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    );
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(screen.getByText('Loading Helm releases')).toBeInTheDocument();
  });

  it('lists a release with its chart, revision and age', async () => {
    stub({ releases: [release()] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(await screen.findAllByText('podinfo')).toHaveLength(2);
    expect(screen.getByText('demo')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
    expect(screen.getByText('deployed')).toBeInTheDocument();
    expect(screen.getByText('1h')).toBeInTheDocument();
  });

  it('keeps the chart version in its own column, apart from the app version', async () => {
    stub({ releases: [release({ latest: '6.9.3', outdated: true, appVersion: '1.2.3' })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    await screen.findByText('demo');
    const cells = screen.getAllByRole('cell').map((cell) => cell.textContent);

    expect(cells.slice(0, 6)).toEqual([
      'podinfo',
      'demo',
      'podinfo',
      '6.9.2',
      '6.9.3 a newer chart version is available',
      '1.2.3',
    ]);
  });

  it('dashes out a chart version the release does not carry', async () => {
    stub({ releases: [release({ chartVersion: '' })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    await screen.findByText('demo');
    const cells = screen.getAllByRole('cell').map((cell) => cell.textContent);

    expect(cells.slice(2, 4)).toEqual(['podinfo', '-']);
  });

  it('marks a chart it could not read at all', async () => {
    stub({ releases: [release({ chart: '', chartVersion: '', appVersion: '' })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    await screen.findByText('demo');
    expect(screen.getAllByText('-')).toHaveLength(4);
  });

  it('shows the newest chart version the repositories offer', async () => {
    stub({ releases: [release({ latest: '7.1.0', outdated: true })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    const latest = await screen.findByText('7.1.0');
    expect(latest).toHaveClass('text-warn');
    expect(latest).toHaveTextContent('a newer chart version is available');
  });

  it('marks a release that already runs the newest chart', async () => {
    stub({ releases: [release({ latest: '6.9.2', outdated: false })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    await screen.findAllByText('podinfo');
    const latest = screen.getByText(/up to date/).closest('td');
    expect(latest).toHaveClass('text-fg-muted');
    expect(latest).toHaveTextContent('6.9.2');
  });

  it('says nothing about a chart no repository knows', async () => {
    stub({ releases: [release()] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    await screen.findAllByText('podinfo');
    expect(screen.getByText(/no chart repository knows this chart/)).toBeInTheDocument();
  });

  it('colours a failed release apart from a deployed one', async () => {
    stub({
      releases: [
        release({ name: 'good', status: 'deployed' }),
        release({ name: 'bad', status: 'failed' }),
        release({ name: 'busy', status: 'pending-upgrade' }),
        release({ name: 'old', status: 'superseded' }),
      ],
    });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(await screen.findByText('failed')).toHaveClass('text-error');
    expect(screen.getByText('deployed')).toHaveClass('text-ok');
    expect(screen.getByText('pending-upgrade')).toHaveClass('text-warn');
    expect(screen.getByText('superseded')).toHaveClass('text-fg-muted');
  });

  it('says an empty status is unknown', async () => {
    stub({ releases: [release({ status: '' })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(await screen.findByText('unknown')).toBeInTheDocument();
  });

  it('filters by name, namespace and chart', async () => {
    const user = userEvent.setup();
    stub({
      releases: [
        release({ name: 'podinfo', namespace: 'demo', chart: 'podinfo' }),
        release({ name: 'grafana', namespace: 'monitoring', chart: 'grafana' }),
      ],
    });
    render(<HelmReleases onSelectResource={vi.fn()} />);
    await screen.findAllByText('grafana');

    await user.type(screen.getByLabelText('Filter releases'), 'monitor');

    expect(screen.getAllByText('grafana')).toHaveLength(2);
    expect(screen.queryByText('podinfo')).not.toBeInTheDocument();
    expect(screen.getByText('1 of 2')).toBeInTheDocument();
  });

  it('says when the filter matches nothing', async () => {
    const user = userEvent.setup();
    stub({ releases: [release()] });
    render(<HelmReleases onSelectResource={vi.fn()} />);
    await screen.findAllByText('podinfo');

    await user.type(screen.getByLabelText('Filter releases'), 'nothing-like-this');

    expect(screen.getByText('Nothing matches that filter.')).toBeInTheDocument();
  });

  it('says when the cluster has no releases at all', async () => {
    stub({ releases: [] });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(await screen.findByText('No Helm releases in this cluster.')).toBeInTheDocument();
  });

  it('carries a partial-data warning from the server', async () => {
    stub({
      releases: [release()],
      error: '1 release payloads could not be read; their name and status come from the labels',
    });
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(await screen.findByText(/could not be read/)).toBeInTheDocument();
    expect(screen.getByText('Partial data.')).toBeInTheDocument();
  });

  it('shows the failure when nothing loaded at all', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('secrets is forbidden')));
    render(<HelmReleases onSelectResource={vi.fn()} />);

    expect(await screen.findByText('secrets is forbidden')).toBeInTheDocument();
    expect(screen.getByText('Helm releases could not be loaded')).toBeInTheDocument();
  });

  it('opens the detail for the release whose name was clicked', async () => {
    const user = userEvent.setup();
    stub({ releases: [release({ name: 'podinfo' }), release({ name: 'grafana' })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);
    await screen.findAllByText('podinfo');

    await user.click(screen.getByRole('button', { name: 'grafana' }));

    expect(screen.getByTestId('release-detail')).toHaveTextContent('detail for grafana');
  });

  it('highlights the row whose detail is open', async () => {
    const user = userEvent.setup();
    stub({ releases: [release({ name: 'podinfo' })] });
    render(<HelmReleases onSelectResource={vi.fn()} />);
    await screen.findAllByText('podinfo');

    await user.click(screen.getByRole('button', { name: 'podinfo' }));

    const row = screen.getByRole('button', { name: 'podinfo' }).closest('tr');
    expect(row?.className).toContain('bg-surface-active');
  });

  it('closes the detail again', async () => {
    const user = userEvent.setup();
    stub({ releases: [release()] });
    render(<HelmReleases onSelectResource={vi.fn()} />);
    await screen.findAllByText('podinfo');
    await user.click(screen.getByRole('button', { name: 'podinfo' }));

    await user.click(screen.getByRole('button', { name: 'close-detail' }));

    expect(screen.queryByTestId('release-detail')).not.toBeInTheDocument();
  });

  it('keeps the last good answer and offers a retry when a later poll fails', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve({ releases: [release()] }) })
      .mockRejectedValue(new Error('helm is down'));
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers();
    render(<HelmReleases onSelectResource={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getAllByText('podinfo').length).toBeGreaterThan(0);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000);
    });

    expect(screen.getAllByText('podinfo').length).toBeGreaterThan(0);
    expect(screen.getByText('Helm releases stopped updating.')).toBeInTheDocument();
    const before = fetchMock.mock.calls.length;
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
  });
});

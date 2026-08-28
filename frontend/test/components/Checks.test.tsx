import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Checks from '../../src/components/Checks';
import type { CheckFinding, CheckGroup, CheckReport } from '../../src/lib/types';

function makeFinding(name: string, extra: Partial<CheckFinding> = {}): CheckFinding {
  return {
    object: { group: 'apps', version: 'v1', resource: 'deployments', namespace: 'apps', name },
    kind: 'Deployment',
    container: 'app',
    detail: 'securityContext.privileged is true',
    ...extra,
  };
}

function makeGroup(id: string, extra: Partial<CheckGroup> = {}): CheckGroup {
  return {
    id,
    title: 'Privileged containers',
    category: 'security',
    severity: 'high',
    wrong: 'a privileged container holds every capability on the node',
    remedy: 'remove securityContext.privileged',
    findings: [],
    ...extra,
  };
}

function stub(body: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve({ ok, status: ok ? 200 : 500, json: () => Promise.resolve(body) })),
  );
}

function show(report: Partial<CheckReport>) {
  const onOpen = vi.fn();
  stub({ groups: [], scanned: 0, ...report });
  render(<Checks onOpen={onOpen} />);
  return { onOpen };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Checks', () => {
  it('says it is loading before the first answer', () => {
    stub({ groups: [], scanned: 0 });

    render(<Checks onOpen={vi.fn()} />);

    expect(screen.getByText(/Loading the cluster audit/)).toBeInTheDocument();
  });

  it('reports a request that never succeeded', async () => {
    stub({ message: 'no cluster' }, false);

    render(<Checks onOpen={vi.fn()} />);

    expect(await screen.findByText('no cluster')).toBeInTheDocument();
  });

  it('counts what it scanned and what it found', async () => {
    show({
      scanned: 12,
      groups: [makeGroup('privileged-containers', { findings: [makeFinding('api')] })],
    });

    expect(await screen.findByText('1 findings across 12 workloads')).toBeInTheDocument();
  });

  it('lists a check under its category with its severity and count', async () => {
    show({
      scanned: 3,
      groups: [
        makeGroup('privileged-containers', { findings: [makeFinding('api')] }),
        makeGroup('image-latest', {
          title: 'Image tagged :latest',
          category: 'reliability',
          severity: 'medium',
        }),
      ],
    });

    expect(await screen.findByText('Security')).toBeInTheDocument();
    expect(screen.getByText('Reliability')).toBeInTheDocument();
    expect(screen.getByText('high')).toBeInTheDocument();
    expect(screen.getByText('clean')).toBeInTheDocument();
  });

  it('leaves out a category no check belongs to', async () => {
    show({ groups: [makeGroup('privileged-containers')] });

    await screen.findByText('Security');
    expect(screen.queryByText('Efficiency')).not.toBeInTheDocument();
  });

  it('opens a check to show what is wrong, what to do and where', async () => {
    show({
      scanned: 1,
      groups: [
        makeGroup('privileged-containers', {
          findings: [makeFinding('api', { patch: 'spec:\n  privileged: false\n' })],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText(/holds every capability/)).toBeInTheDocument();
    expect(screen.getByText(/remove securityContext.privileged/)).toBeInTheDocument();
    expect(screen.getByText('Deployment · apps/api · container app')).toBeInTheDocument();
    expect(screen.getByText(/privileged: false/)).toBeInTheDocument();
  });

  it('lists a finding that landed on the workload rather than a container', async () => {
    show({
      groups: [
        makeGroup('host-namespaces', {
          findings: [
            makeFinding('api', { container: undefined, detail: 'shares the host namespaces' }),
          ],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('Deployment · apps/api')).toBeInTheDocument();
  });

  it('closes a check that was open', async () => {
    show({ groups: [makeGroup('privileged-containers', { findings: [makeFinding('api')] })] });

    const header = await screen.findByRole('button', { name: /Privileged containers/ });
    await userEvent.click(header);
    await userEvent.click(header);

    expect(screen.queryByText(/holds every capability/)).not.toBeInTheDocument();
  });

  it('shows no patch when the fix needs a value only the owner knows', async () => {
    show({ groups: [makeGroup('probes-missing', { findings: [makeFinding('api')] })] });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.queryByRole('figure')).not.toBeInTheDocument();
    expect(document.querySelector('pre')).toBeNull();
  });

  it('opens the object a finding landed on', async () => {
    const { onOpen } = show({
      groups: [makeGroup('privileged-containers', { findings: [makeFinding('api')] })],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /apps\/api/ }));

    expect(onOpen).toHaveBeenCalledWith(
      { group: 'apps', version: 'v1', resource: 'deployments', namespace: 'apps', name: 'api' },
      'Deployment',
    );
  });

  it('refuses to open a check that found nothing', async () => {
    show({ groups: [makeGroup('privileged-containers')] });

    const header = await screen.findByRole('button', { name: /Privileged containers/ });

    expect(header).toBeDisabled();
  });

  it('says why a check could not run', async () => {
    show({
      groups: [
        makeGroup('requests-far-above-usage', {
          title: 'Requests far above measured usage',
          category: 'efficiency',
          severity: 'low',
          skipped: 'metrics-server did not answer',
        }),
      ],
    });

    expect(await screen.findByText('metrics-server did not answer')).toBeInTheDocument();
    expect(screen.getByText('no data')).toBeInTheDocument();
  });

  it('shows the framework a check comes from', async () => {
    show({
      groups: [makeGroup('privileged-containers', { frameworks: ['PSS baseline', 'NSA/CISA'] })],
    });

    expect(await screen.findByText('PSS baseline')).toBeInTheDocument();
    expect(screen.getByText('NSA/CISA')).toBeInTheDocument();
  });

  it('passes on a partial read of the cluster', async () => {
    show({ groups: [], error: 'deployments.apps is forbidden' });

    expect(await screen.findByText('deployments.apps is forbidden')).toBeInTheDocument();
  });

  it('says when the audit stopped updating', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, status: 200, json: () => Promise.resolve({ groups: [] }) })
      .mockResolvedValue({ ok: false, status: 500, json: () => Promise.resolve({}) });
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers({ shouldAdvanceTime: true });

    render(<Checks onOpen={vi.fn()} />);
    await screen.findByText('0 findings across 0 workloads');
    await vi.advanceTimersByTimeAsync(20000);

    await waitFor(() => {
      expect(screen.getByText(/stopped updating/)).toBeInTheDocument();
    });
    vi.useRealTimers();
  });
});

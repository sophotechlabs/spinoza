import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { Issue, IssueQueue as Queue } from '../../src/lib/types';
import IssueQueue from '../../src/components/IssueQueue';

function child(name: string) {
  return {
    object: { group: '', version: 'v1', resource: 'pods', namespace: 'web', name },
    kind: 'Pod',
    severity: 'fatal' as const,
    detail: `container app in ${name} keeps exiting`,
    since: '2026-08-28T11:00:00Z',
  };
}

function issue(patch: Partial<Issue> = {}): Issue {
  return {
    id: 'pod-startup/uid-web',
    severity: 'fatal',
    detector: 'pod-startup',
    title: 'CrashLoopBackOff',
    detail: 'container app keeps exiting with exit code 1',
    action: 'read the container logs',
    object: {
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'web',
      name: 'api',
    },
    kind: 'Deployment',
    since: '2026-08-28T11:00:00Z',
    folded: 0,
    ...patch,
  };
}

function queue(patch: Partial<Queue> = {}): Queue {
  return { rows: [issue()], dropped: 0, ...patch };
}

function stub(data: Queue): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(data) }),
  );
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('IssueQueue', () => {
  it('waits for the first answer', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    );

    render(<IssueQueue />);

    expect(screen.getByText('Loading the issue queue')).toBeInTheDocument();
  });

  it('says so when the request never lands', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));

    render(<IssueQueue />);

    expect(await screen.findByText(/network down/)).toBeInTheDocument();
  });

  it('shows the row, what to do about it and where it is', async () => {
    stub(queue());

    render(<IssueQueue />);

    expect(await screen.findByText('CrashLoopBackOff')).toBeInTheDocument();
    expect(screen.getByText('container app keeps exiting with exit code 1')).toBeInTheDocument();
    expect(screen.getByText('read the container logs')).toBeInTheDocument();
    expect(screen.getByText('api')).toBeInTheDocument();
    expect(screen.getByText(/Deployment/)).toBeInTheDocument();
  });

  it('says the cluster is fine when nothing is broken', async () => {
    stub(queue({ rows: [] }));

    render(<IssueQueue />);

    expect(
      await screen.findByText('Nothing is broken in this cluster right now.'),
    ).toBeInTheDocument();
  });

  it('tallies the rows by how bad they are', async () => {
    stub(
      queue({
        rows: [
          issue(),
          issue({ id: 'two', severity: 'degraded' }),
          issue({ id: 'three', severity: 'warning' }),
        ],
      }),
    );

    render(<IssueQueue />);

    expect(await screen.findByText('1 broken')).toBeInTheDocument();
    expect(screen.getByText('1 degraded')).toBeInTheDocument();
    expect(screen.getByText('1 warning')).toBeInTheDocument();
  });

  it('opens the fold to show what it folded', async () => {
    stub(queue({ rows: [issue({ folded: 2, children: [child('api-1'), child('api-2')] })] }));

    render(<IssueQueue />);

    const toggle = await screen.findByRole('button', { expanded: false });
    fireEvent.click(toggle);

    expect(await screen.findByText('container app in api-1 keeps exiting')).toBeInTheDocument();
    expect(screen.getByText('container app in api-2 keeps exiting')).toBeInTheDocument();
  });

  it('closes the fold again', async () => {
    stub(queue({ rows: [issue({ folded: 1, children: [child('api-1')] })] }));

    render(<IssueQueue />);

    const toggle = await screen.findByRole('button', { expanded: false });
    fireEvent.click(toggle);
    expect(await screen.findByText('container app in api-1 keeps exiting')).toBeInTheDocument();

    fireEvent.click(toggle);
    await waitFor(() => {
      expect(screen.queryByText('container app in api-1 keeps exiting')).not.toBeInTheDocument();
    });
  });

  it('says how many folded children it left out', async () => {
    stub(queue({ rows: [issue({ folded: 200, children: [child('api-1')] })] }));

    render(<IssueQueue />);

    fireEvent.click(await screen.findByRole('button', { expanded: false }));

    expect(await screen.findByText('and 199 more not listed, out of 200')).toBeInTheDocument();
  });

  it('shows the change that came before the failure', async () => {
    stub(queue({ rows: [issue({ change: 'revision 4', changedAt: '2026-08-28T10:00:00Z' })] }));

    render(<IssueQueue />);

    expect(await screen.findByText(/revision 4/)).toBeInTheDocument();
  });

  it('shows a change with no timestamp on its own', async () => {
    stub(queue({ rows: [issue({ change: 'revision 4' })] }));

    render(<IssueQueue />);

    expect(await screen.findByText('revision 4')).toBeInTheDocument();
  });

  it('leaves the change column blank when there is nothing to say', async () => {
    stub(queue());

    render(<IssueQueue />);

    expect(await screen.findByText('-')).toBeInTheDocument();
  });

  it('says when a row is a guess', async () => {
    stub(queue({ rows: [issue({ uncertain: true })] }));

    render(<IssueQueue />);

    expect(await screen.findByText('(a guess)')).toBeInTheDocument();
  });

  it('reports what the queue could not read', async () => {
    stub(queue({ error: '1 of 4 resource types could not be listed' }));

    render(<IssueQueue />);

    expect(await screen.findByText(/could not be listed/)).toBeInTheDocument();
  });

  it('says how many rows it dropped at the cap', async () => {
    stub(queue({ dropped: 12 }));

    render(<IssueQueue />);

    expect(await screen.findByText(/12 more rows are not shown/)).toBeInTheDocument();
  });

  it('hands the object back when a row is clicked', async () => {
    stub(queue());
    const onSelect = vi.fn();

    render(<IssueQueue onSelect={onSelect} />);

    fireEvent.click(await screen.findByText('api'));

    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ resource: 'deployments', name: 'api' }),
    );
  });

  it('does not fall over when a row is clicked with nowhere to send it', async () => {
    stub(queue());

    render(<IssueQueue />);

    fireEvent.click(await screen.findByText('api'));

    expect(screen.getByText('CrashLoopBackOff')).toBeInTheDocument();
  });

  it('shows a row for an object with no namespace', async () => {
    stub(
      queue({
        rows: [
          issue({
            kind: 'Node',
            object: { group: '', version: 'v1', resource: 'nodes', namespace: '', name: 'node-a' },
          }),
        ],
      }),
    );

    render(<IssueQueue />);

    expect(await screen.findByText('node-a')).toBeInTheDocument();
  });

  it('keeps the last good answer and offers a retry when a later poll fails', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(queue()) })
      .mockRejectedValue(new Error('issues are down'));
    vi.stubGlobal('fetch', fetchMock);
    vi.useFakeTimers();
    render(<IssueQueue />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('CrashLoopBackOff')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByText('CrashLoopBackOff')).toBeInTheDocument();
    expect(screen.getByText('The issue queue stopped updating.')).toBeInTheDocument();
    const before = fetchMock.mock.calls.length;
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
  });

  it('offers no fold control on a row that folded nothing', async () => {
    stub(queue());

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');

    expect(screen.queryByRole('button', { expanded: false })).not.toBeInTheDocument();
  });

  it('names the fold control for a reader who cannot see the arrow', async () => {
    stub(queue({ rows: [issue({ folded: 2, children: [child('api-1'), child('api-2')] })] }));

    render(<IssueQueue />);

    expect(
      await screen.findByRole('button', { name: 'Show the 2 objects folded under api' }),
    ).toBeInTheDocument();
  });

  it('keeps a long detail readable and puts the whole of it within reach', async () => {
    const detail = 'container app: '.padEnd(400, 'x');
    stub(queue({ rows: [issue({ detail })] }));

    render(<IssueQueue />);

    expect(await screen.findByTitle(detail)).toBeInTheDocument();
  });

  it('stays quiet while the view is hidden', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(<IssueQueue active={false} />);

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { Issue, IssueQueue as Queue } from '../../src/lib/types';
import IssueQueue from '../../src/components/IssueQueue';
import { useClustersStore } from '../../src/store/clusters';
import { useIssuesStore } from '../../src/store/issues';
import { MK1, MK2, showing } from '../helpers-clusters';

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

function stubEach(own: Queue, fleet: Queue): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      asked.push(url);
      const body = url.includes('/fleet') ? fleet : own;
      return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
    }),
  );
  return asked;
}

function twoOpen(): void {
  act(() => {
    showing(MK1);
  });
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  useClustersStore.getState().reset();
  useIssuesStore.setState({ fleet: false });
});

function pagedStub(pages: Record<string, Queue | undefined>): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      asked.push(url);
      const after = new URL(url, 'http://localhost').searchParams.get('after') ?? '';
      const body = pages[after];
      if (body === undefined) {
        return Promise.resolve({ ok: false, status: 404, json: () => Promise.resolve({}) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
    }),
  );
  return asked;
}

describe('IssueQueue', () => {
  it('leaves the rest of the queue behind a button until it is asked for', async () => {
    pagedStub({
      '': queue({ rows: [issue()], next: 'cursor-1' }),
      'cursor-1': queue({ rows: [issue({ id: 'second', title: 'ImagePullBackOff' })] }),
    });

    render(<IssueQueue />);
    expect(await screen.findByText('CrashLoopBackOff')).toBeInTheDocument();
    expect(screen.queryByText('ImagePullBackOff')).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));

    expect(await screen.findByText('ImagePullBackOff')).toBeInTheDocument();
    expect(screen.getByText('CrashLoopBackOff')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument();
    });
  });

  it('offers nothing more when the queue fits on one page', async () => {
    stub(queue());

    render(<IssueQueue />);

    expect(await screen.findByText('CrashLoopBackOff')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument();
  });

  it('says so when the rest of the queue will not load', async () => {
    pagedStub({ '': queue({ rows: [issue()], next: 'cursor-1' }) });

    render(<IssueQueue />);
    expect(await screen.findByText('CrashLoopBackOff')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Show more' }));

    expect(await screen.findByText(/issues request failed with status 404/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show more' })).toBeInTheDocument();
  });

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

  it('offers nothing to switch to when only one cluster is open', async () => {
    stub(queue());

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');

    expect(screen.queryByLabelText('Every open cluster')).not.toBeInTheDocument();
  });

  it('offers the whole fleet once a second cluster is open', async () => {
    stub(queue());
    twoOpen();

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');

    expect(screen.getByLabelText('Every open cluster')).not.toBeChecked();
  });

  it('asks for the merged queue once the fleet is asked for', async () => {
    const asked = stubEach(queue(), queue({ rows: [issue({ id: 'other', cluster: MK2 })] }));
    twoOpen();

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));

    await waitFor(() => {
      expect(asked.some((url) => url.includes('/api/issues/fleet'))).toBe(true);
    });
  });

  it('says which cluster each row is on', async () => {
    stubEach(queue(), queue({ rows: [issue({ cluster: MK2 })] }));
    twoOpen();

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));

    expect(await screen.findByText('p-mk2')).toBeInTheDocument();
  });

  it('leaves the cluster blank for a row from one that is no longer open', async () => {
    stubEach(queue(), queue({ rows: [issue({ cluster: 'https://gone:6443' })] }));
    twoOpen();

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));

    expect(await screen.findByText('unknown')).toBeInTheDocument();
  });

  it('sends a row on another cluster to whoever can switch there', async () => {
    stubEach(queue(), queue({ rows: [issue({ cluster: MK2 })] }));
    twoOpen();
    const onSelectOn = vi.fn();

    render(<IssueQueue onSelectOn={onSelectOn} />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));
    await screen.findByText('p-mk2');
    fireEvent.click(screen.getByText('api'));

    expect(onSelectOn).toHaveBeenCalledWith(
      MK2,
      expect.objectContaining({ resource: 'deployments', name: 'api' }),
    );
  });

  it('selects a row that says no cluster the ordinary way', async () => {
    stubEach(queue(), queue({ rows: [issue()] }));
    twoOpen();
    const onSelect = vi.fn();
    const onSelectOn = vi.fn();

    render(<IssueQueue onSelect={onSelect} onSelectOn={onSelectOn} />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));
    await screen.findByText('unknown');
    fireEvent.click(screen.getByText('api'));

    expect(onSelectOn).not.toHaveBeenCalled();
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ name: 'api' }));
  });

  it('says the whole fleet is fine when nothing is broken anywhere', async () => {
    stubEach(queue(), queue({ rows: [] }));
    twoOpen();

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));

    expect(
      await screen.findByText('Nothing is broken in any open cluster right now.'),
    ).toBeInTheDocument();
  });

  it('goes back to this cluster when the fleet is turned off again', async () => {
    stubEach(queue(), queue({ rows: [issue({ id: 'elsewhere', cluster: MK2 })] }));
    twoOpen();

    render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');
    const toggle = screen.getByLabelText('Every open cluster');
    fireEvent.click(toggle);
    await screen.findByText('p-mk2');

    fireEvent.click(toggle);

    await waitFor(() => {
      expect(screen.queryByText('p-mk2')).not.toBeInTheDocument();
    });
  });

  it('keeps the fleet in view when the cluster underneath it changes', async () => {
    stubEach(queue(), queue({ rows: [issue({ cluster: MK2 })] }));
    twoOpen();
    const { unmount } = render(<IssueQueue />);
    await screen.findByText('CrashLoopBackOff');
    fireEvent.click(screen.getByLabelText('Every open cluster'));
    await screen.findByText('p-mk2');
    unmount();

    render(<IssueQueue />);

    expect(await screen.findByLabelText('Every open cluster')).toBeChecked();
  });

  it('stays quiet while the view is hidden', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    render(<IssueQueue active={false} />);

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

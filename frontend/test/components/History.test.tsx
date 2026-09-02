import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import History from '../../src/components/History';
import type { HistoryEntry } from '../../src/lib/types';
import { useClustersStore } from '../../src/store/clusters';
import { useToastsStore } from '../../src/store/toasts';
import { MK1, MK2, showing } from '../helpers-clusters';
import { bumpClusterEpoch } from '../../src/store/cluster';

function entry(extra: Partial<HistoryEntry> = {}): HistoryEntry {
  return {
    id: 1,
    source: 'action',
    at: '2026-08-29T09:30:00Z',
    verb: 'delete',
    actor: 'local',
    name: 'web',
    outcome: 'done',
    ...extra,
  };
}

function stub(body: unknown, ok = true, status = 200) {
  const fetcher = vi.fn((url: string, init?: RequestInit) => {
    void url;
    void init;
    return Promise.resolve({
      ok,
      status,
      json: () => Promise.resolve(body),
      text: () => Promise.resolve(JSON.stringify(body)),
    });
  });
  vi.stubGlobal('fetch', fetcher);
  return fetcher;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useClustersStore.getState().reset();
  useToastsStore.setState({ toasts: [], history: [] });
});

describe('History', () => {
  it('shows what spinoza did', async () => {
    stub({
      entries: [
        entry({
          verb: 'scale',
          actor: 'alice@example.com',
          kind: 'Deployment',
          resource: 'deployments',
          namespace: 'default',
          detail: 'to 3 replicas',
        }),
      ],
    });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('Deployment web')).toBeTruthy();
    expect(screen.getByText('scale')).toBeTruthy();
    expect(screen.getByText('alice@example.com')).toBeTruthy();
    expect(screen.getByText('default')).toBeTruthy();
    expect(screen.getByText('to 3 replicas')).toBeTruthy();
    expect(screen.getByText('Done')).toBeTruthy();
  });

  it('marks actions recorded before actors were available', async () => {
    stub({ entries: [entry({ actor: undefined })] });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('unknown')).toBeTruthy();
  });

  it('says so when nothing has been done yet', async () => {
    stub({ entries: [] });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('There is nothing here yet.')).toBeTruthy();
  });

  it('says when it is showing only the newest page', async () => {
    stub({ entries: [entry()], more: true });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/showing the newest/)).toBeTruthy();
  });

  it('does not claim a page was trimmed when it was not', async () => {
    stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('web');
    expect(screen.queryByText(/showing the newest/)).toBeNull();
  });

  it('warns when spinoza is not recording', async () => {
    stub({ entries: [], reason: 'the history file is read-only' });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('the history file is read-only')).toBeTruthy();
  });

  it('opens the object a row names', async () => {
    stub({
      entries: [
        entry({ group: 'apps', version: 'v1', resource: 'deployments', kind: 'Deployment' }),
      ],
    });
    const onOpen = vi.fn();

    render(<History onOpen={onOpen} />);
    await userEvent.click(await screen.findByRole('button', { name: 'Deployment web' }));

    expect(onOpen).toHaveBeenCalledWith(
      expect.objectContaining({ resource: 'deployments', name: 'web' }),
    );
  });

  it('leaves a row with no resource unclickable', async () => {
    stub({ entries: [entry({ kind: 'Release' })] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('Release web');
    expect(screen.queryByRole('button', { name: 'Release web' })).toBeNull();
  });

  it('shows a spinner before the first answer', () => {
    stub({ entries: [] });

    render(<History onOpen={vi.fn()} />);

    expect(screen.getByRole('status')).toBeTruthy();
  });

  it('reports a first load that failed', async () => {
    stub({ message: 'the database went away' }, false, 500);

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('the database went away')).toBeTruthy();
  });

  it('falls back to its own words when the server explained nothing', async () => {
    stub({}, false, 500);

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/history request failed with status 500/)).toBeTruthy();
  });

  it('keeps the rows on screen and warns when a refresh fails', async () => {
    let at = -1;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        void url;
        at += 1;
        const failing = init?.method !== 'DELETE' && at > 0;
        return Promise.resolve({
          ok: !failing,
          status: failing ? 500 : 200,
          json: () =>
            Promise.resolve(
              failing ? { message: 'the database went away' } : { entries: [entry()] },
            ),
          text: () => Promise.resolve(''),
        });
      }),
    );

    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    expect(await screen.findByText(/the database went away/)).toBeTruthy();
    expect(screen.getByText('web')).toBeTruthy();
  });

  it('clears the history when asked', async () => {
    const fetcher = stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    await waitFor(() => {
      expect(fetcher.mock.calls.some((call) => call[1]?.method === 'DELETE')).toBe(true);
    });
  });

  it('says so when the clear itself was refused', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        void url;
        const clearing = init?.method === 'DELETE';
        return Promise.resolve({
          ok: !clearing,
          status: clearing ? 503 : 200,
          json: () =>
            Promise.resolve(
              clearing ? { message: 'spinoza is not recording' } : { entries: [entry()] },
            ),
          text: () => Promise.resolve(''),
        });
      }),
    );

    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await userEvent.click(screen.getByRole('button', { name: 'Clear' }));

    await waitFor(() => {
      expect(screen.getByRole('button', { name: 'Clear' }).hasAttribute('disabled')).toBe(false);
    });
    expect(screen.getByText('web')).toBeTruthy();
  });

  it('drops a clear completion after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let finishClear: (response: unknown) => void = () => undefined;
    const clearing = new Promise((resolve) => {
      finishClear = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return clearing;
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()] }),
        });
      }),
    );
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await user.click(screen.getByRole('button', { name: 'Clear' }));

    act(() => {
      bumpClusterEpoch();
    });
    await screen.findByText('web');

    await act(async () => {
      finishClear({ ok: true, status: 200, json: () => Promise.resolve({}) });
      await Promise.resolve();
    });

    expect(useToastsStore.getState().toasts).toEqual([]);
    expect(screen.getByRole('button', { name: 'Clear' })).toBeEnabled();
  });

  it('drops a clear failure after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let failClear: (reason: unknown) => void = () => undefined;
    const clearing = new Promise((_resolve, reject) => {
      failClear = reject;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: RequestInit) => {
        if (init?.method === 'DELETE') {
          return clearing;
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()] }),
        });
      }),
    );
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await user.click(screen.getByRole('button', { name: 'Clear' }));

    act(() => {
      bumpClusterEpoch();
    });
    await screen.findByText('web');
    await act(async () => {
      failClear(new Error('old cluster failed'));
      await Promise.resolve();
    });

    expect(useToastsStore.getState().toasts).toEqual([]);
    expect(screen.getByRole('button', { name: 'Clear' })).toBeEnabled();
  });

  it('cannot be cleared when there is nothing to clear', async () => {
    stub({ entries: [] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('There is nothing here yet.');
    expect(screen.getByRole('button', { name: 'Clear' }).hasAttribute('disabled')).toBe(true);
  });
  it('shows what spinoza did and what the cluster did together', async () => {
    stub({
      entries: [
        entry(),
        entry({ id: 2, source: 'change', verb: 'changed', name: 'web-1', detail: '1/1 · Running' }),
      ],
    });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('web-1')).toBeTruthy();
    expect(screen.getByText('changed')).toBeTruthy();
    expect(screen.getByText('1/1 · Running')).toBeTruthy();
  });

  it('asks for only what changed when that is what is picked', async () => {
    const calls = stub({ entries: [entry()] });
    const user = userEvent.setup();
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.selectOptions(screen.getByLabelText('What to show'), 'change');

    await waitFor(() => {
      expect(calls.mock.calls.some((call) => call[0].includes('source=change'))).toBe(true);
    });
  });

  it('leaves the outcome blank on something the cluster did', async () => {
    stub({ entries: [entry({ source: 'change', verb: 'added', outcome: 'done' })] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('appeared');
    expect(screen.queryByText('Done')).toBeNull();
  });

  it('says when changes came in faster than they could be kept', async () => {
    stub({ entries: [entry()], dropped: 40 });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/40 changes came in faster/)).toBeTruthy();
  });

  it('says the cluster has been quiet when only changes are shown', async () => {
    const user = userEvent.setup();
    stub({ entries: [] });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('There is nothing here yet.');

    await user.selectOptions(screen.getByLabelText('What to show'), 'change');

    expect(
      await screen.findByText(
        'Nothing has changed on this cluster since spinoza started watching it.',
      ),
    ).toBeTruthy();
  });

  it('says spinoza has done nothing when only its own actions are shown', async () => {
    const user = userEvent.setup();
    stub({ entries: [] });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('There is nothing here yet.');

    await user.selectOptions(screen.getByLabelText('What to show'), 'action');

    expect(
      await screen.findByText('Spinoza has not changed anything on this cluster yet.'),
    ).toBeTruthy();
  });
  it('offers nothing to record when no cluster is open', async () => {
    stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('web');
    expect(screen.queryByLabelText('What to record')).toBeNull();
  });

  it('asks the server to start recording the open cluster', async () => {
    const user = userEvent.setup();
    const calls = stub({ entries: [entry()] });
    act(() => {
      showing(MK1);
    });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.selectOptions(screen.getByLabelText('What to record'), 'workloads');

    await waitFor(() => {
      expect(calls.mock.calls.some((call) => call[0].includes('kinds=workloads'))).toBe(true);
    });
  });

  it('says so when the server will not record it', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/clusters/timeline')) {
          return Promise.resolve({
            ok: false,
            status: 503,
            json: () => Promise.resolve({ message: 'nowhere to keep that' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()] }),
        });
      }),
    );
    act(() => {
      showing(MK1);
    });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.selectOptions(screen.getByLabelText('What to record'), 'workloads');

    await waitFor(() => {
      const said = useToastsStore.getState().toasts.map((toast) => toast.message);
      expect(said.some((message) => message.includes('Changing what is recorded'))).toBe(true);
    });
  });

  it('drops a recording failure after another cluster is selected', async () => {
    const user = userEvent.setup();
    let failRecording: (reason?: unknown) => void = () => undefined;
    const recording = new Promise((_resolve, reject) => {
      failRecording = reject;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/clusters/timeline')) {
          return recording;
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()] }),
        });
      }),
    );
    act(() => {
      showing(MK1);
    });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');
    await user.selectOptions(screen.getByLabelText('What to record'), 'workloads');

    act(() => {
      showing(MK2);
    });
    await act(async () => {
      failRecording(new Error('the old cluster stopped recording'));
      await Promise.resolve();
    });

    expect(useToastsStore.getState().toasts).toEqual([]);
    expect(screen.getByLabelText('What to record')).toBeEnabled();
  });
  it('folds repeated changes to one object into a single row', async () => {
    stub({
      entries: [
        entry({ id: 3, source: 'change', verb: 'changed', name: 'calico-node', detail: '94/94' }),
        entry({ id: 2, source: 'change', verb: 'changed', name: 'calico-node', detail: '94/93' }),
      ],
    });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/changed 2 times/)).toBeTruthy();
  });

  it('shows what a change moved from', async () => {
    stub({
      entries: [entry({ source: 'change', verb: 'changed', detail: '1/3', was: '3/3' })],
    });

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText(/3\/3 →/)).toBeTruthy();
  });

  it('offers nothing to roll up when one cluster is open', async () => {
    stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('web');
    expect(screen.queryByLabelText('Every open cluster')).toBeNull();
  });

  it('rolls up every open cluster when asked', async () => {
    const user = userEvent.setup();
    const calls = stub({ entries: [entry()] });
    act(() => {
      showing(MK1);
    });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByLabelText('Every open cluster'));

    await waitFor(() => {
      expect(calls.mock.calls.some((call) => call[0].includes('fleet=true'))).toBe(true);
    });
  });

  it('names the cluster on every row once it is rolling up', async () => {
    const user = userEvent.setup();
    stub({ entries: [entry({ cluster: MK1 })] });
    act(() => {
      showing(MK1);
    });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByLabelText('Every open cluster'));

    expect(await screen.findByText('Cluster')).toBeTruthy();
    expect(await screen.findByText('p-mk1')).toBeTruthy();
  });

  it('says so when a row is on a cluster that is gone', async () => {
    const user = userEvent.setup();
    stub({ entries: [entry({ cluster: 'https://gone:6443' })] });
    act(() => {
      showing(MK1);
    });
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByLabelText('Every open cluster'));

    expect(await screen.findByText('unknown')).toBeTruthy();
  });
  it('says what spinoza is holding while it watches', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/memory')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ heapMi: 325, sysMi: 689 }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()] }),
        });
      }),
    );

    render(<History onOpen={vi.fn()} />);

    expect(await screen.findByText('325 MB held')).toBeTruthy();
  });

  it('offers nothing older when the page holds everything', async () => {
    stub({ entries: [entry()] });

    render(<History onOpen={vi.fn()} />);

    await screen.findByText('web');
    expect(screen.queryByRole('button', { name: 'Show older' })).toBeNull();
  });

  it('reaches further back when there is more', async () => {
    const user = userEvent.setup();
    const calls = vi.fn((url: string) => {
      if (url.includes('after=')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry({ id: 9, name: 'older-one' })], next: 8 }),
        });
      }
      if (url.includes('/api/memory')) {
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ entries: [entry()], more: true, next: 40 }),
      });
    });
    vi.stubGlobal('fetch', calls);
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByRole('button', { name: 'Show older' }));

    expect(await screen.findByText('older-one')).toBeTruthy();
    expect(calls.mock.calls.some((call) => call[0].includes('after=40'))).toBe(true);
  });

  it('drops an older page when the history scope changes', async () => {
    const user = userEvent.setup();
    let answerOlder: (body: unknown) => void = () => undefined;
    const older = new Promise((resolve) => {
      answerOlder = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/memory')) {
          return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
        }
        if (url.includes('after=')) {
          return older;
        }
        if (url.includes('source=change')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () =>
              Promise.resolve({
                entries: [entry({ id: 30, source: 'change', name: 'changed-only' })],
              }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()], more: true, next: 40 }),
        });
      }),
    );
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByRole('button', { name: 'Show older' }));
    await user.selectOptions(screen.getByLabelText('What to show'), 'change');
    expect(await screen.findByText('changed-only')).toBeTruthy();

    answerOlder({
      ok: true,
      status: 200,
      json: () => Promise.resolve({ entries: [entry({ id: 9, name: 'older-one' })] }),
    });

    await waitFor(() => {
      expect(screen.queryByText('older-one')).toBeNull();
    });
  });

  it('drops an older-page failure when the history scope changes', async () => {
    const user = userEvent.setup();
    let failOlder: (reason?: unknown) => void = () => undefined;
    const older = new Promise((_resolve, reject) => {
      failOlder = reject;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('/api/memory')) {
          return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
        }
        if (url.includes('after=')) {
          return older;
        }
        if (url.includes('source=change')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () =>
              Promise.resolve({
                entries: [entry({ id: 30, source: 'change', name: 'changed-only' })],
              }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()], more: true, next: 40 }),
        });
      }),
    );
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByRole('button', { name: 'Show older' }));
    await user.selectOptions(screen.getByLabelText('What to show'), 'change');
    expect(await screen.findByText('changed-only')).toBeTruthy();
    await act(async () => {
      failOlder(new Error('the old page failed'));
      await Promise.resolve();
    });

    const said = useToastsStore.getState().toasts.map((toast) => toast.message);
    expect(said.some((message) => message.includes('the old page failed'))).toBe(false);
    expect(screen.getByText('changed-only')).toBeTruthy();
  });

  it('says so when reaching back fails', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.includes('after=')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the store went away' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ entries: [entry()], more: true, next: 40 }),
        });
      }),
    );
    render(<History onOpen={vi.fn()} />);
    await screen.findByText('web');

    await user.click(screen.getByRole('button', { name: 'Show older' }));

    await waitFor(() => {
      const said = useToastsStore.getState().toasts.map((toast) => toast.message);
      expect(said.some((message) => message.includes('Reaching further back'))).toBe(true);
    });
  });
});

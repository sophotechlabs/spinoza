import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ContextPicker from '../../src/components/ContextPicker';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';
import { expireSession } from '../../src/store/session';
import { adoptClusters } from '../../src/store/clusters';

async function openMenu(user: ReturnType<typeof userEvent.setup>): Promise<HTMLElement> {
  const summary = await screen.findByLabelText('Kubernetes context');
  await user.click(summary);
  const menu = summary.parentElement;
  if (menu === null) {
    throw new Error('the context menu has no parent');
  }
  return menu;
}

async function pick(user: ReturnType<typeof userEvent.setup>, name: string, at = 0): Promise<void> {
  await openMenu(user);
  await user.click(screen.getAllByRole('button', { name })[at]);
}

function defaultKubeconfig(names: string[]) {
  return {
    label: '/home/arch/.kube/config',
    path: '',
    removable: false,
    contexts: names.map((name) => ({ name, cluster: name })),
  };
}

function listOf(names: string[], current: string) {
  return {
    current: { kubeconfig: '', name: current },
    kubeconfigs: [defaultKubeconfig(names)],
  };
}

function stubContexts(list: unknown, post?: unknown) {
  const calls: { url: string; method?: string }[] = [];
  const fetchMock = vi.fn((url: string, init?: { method?: string }) => {
    calls.push({ url, method: init?.method });
    if (init?.method === 'POST') {
      if (post === undefined) {
        return Promise.resolve({
          ok: false,
          status: 400,
          json: () => Promise.resolve({ message: 'context "gone" does not exist' }),
        });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(post) });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve(list) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

beforeEach(() => {
  useToastsStore.getState().clear();
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.open = false;
  };
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  useToastsStore.getState().clear();
});

describe('ContextPicker', () => {
  it('lists every context and marks the one in use', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));

    render(<ContextPicker onSwitched={vi.fn()} />);
    await openMenu(user);

    expect(screen.getByLabelText('Kubernetes context')).toHaveTextContent('p-mk2');
    expect(screen.getByRole('button', { name: 'p-mk1' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'p-mk2' })).toHaveAttribute('aria-current', 'true');
  });

  it('opens with the whole list in view, not scrolled to the context in use', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2', 'p-mk3'], 'p-mk3'));

    render(<ContextPicker onSwitched={vi.fn()} />);
    const menu = await openMenu(user);

    expect(menu).toHaveAttribute('open');
    expect(
      within(menu)
        .getAllByRole('button')
        .map((one) => one.textContent),
    ).toEqual(['p-mk1', 'p-mk2', 'p-mk3', 'Manage kubeconfigs']);
  });

  it('groups the contexts under the kubeconfig they came from', async () => {
    stubContexts({
      current: { kubeconfig: '', name: 'p-mk2' },
      kubeconfigs: [
        defaultKubeconfig(['p-mk1', 'p-mk2']),
        {
          label: '/home/arch/.kube/work.yaml',
          path: '/home/arch/.kube/work.yaml',
          removable: true,
          contexts: [{ name: 'staging', cluster: 'work', namespace: 'apps' }],
        },
      ],
    });

    const user = userEvent.setup();
    render(<ContextPicker onSwitched={vi.fn()} />);

    const menu = await openMenu(user);

    expect(within(menu).getByText('/home/arch/.kube/config')).toBeInTheDocument();
    expect(within(menu).getByText('/home/arch/.kube/work.yaml')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'staging' })).toHaveAttribute(
      'title',
      'cluster work, namespace apps',
    );
  });

  it('opens the chosen context as a tab and tells the app to show it', async () => {
    const user = userEvent.setup();
    const calls = stubContexts(
      listOf(['p-mk1', 'p-mk2'], 'p-mk2'),
      listOf(['p-mk1', 'p-mk2'], 'p-mk1'),
    );
    const onSwitched = vi.fn();
    render(<ContextPicker onSwitched={onSwitched} />);

    await pick(user, 'p-mk1');

    await waitFor(() => {
      expect(onSwitched).toHaveBeenCalled();
    });
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.url).toContain('name=p-mk1');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Opened p-mk1' }),
    ]);
  });

  it('names the kubeconfig the chosen context came from', async () => {
    const user = userEvent.setup();
    const second = {
      label: '/home/arch/.kube/work.yaml',
      path: '/home/arch/.kube/work.yaml',
      removable: true,
      contexts: [{ name: 'p-mk1', cluster: 'work' }],
    };
    const list = {
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [defaultKubeconfig(['p-mk1']), second],
    };
    const calls = stubContexts(list, {
      current: { kubeconfig: '/home/arch/.kube/work.yaml', name: 'p-mk1' },
      kubeconfigs: [defaultKubeconfig(['p-mk1']), second],
    });
    render(<ContextPicker onSwitched={vi.fn()} />);

    await pick(user, 'p-mk1', 1);

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST')).toBe(true);
    });
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.url).toContain('kubeconfig=%2Fhome%2Farch%2F.kube%2Fwork.yaml');
  });

  it('reports an open the server refused and keeps the old context', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));
    const onSwitched = vi.fn();
    render(<ContextPicker onSwitched={onSwitched} />);

    await pick(user, 'p-mk1');

    expect(await screen.findByText('context "gone" does not exist')).toBeInTheDocument();
    expect(onSwitched).not.toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'Opening p-mk1: context "gone" does not exist',
      }),
    ]);
  });

  it('shows a plain label when the kubeconfig has no contexts', async () => {
    stubContexts({ current: { kubeconfig: '', name: 'embedded' }, kubeconfigs: [] });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText('embedded')).toBeInTheDocument();
    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('says there is no cluster when nothing is connected', async () => {
    stubContexts({ current: { kubeconfig: '', name: '' }, kubeconfigs: [] });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText('no cluster')).toBeInTheDocument();
  });

  it('keeps the connected context visible when no kubeconfig lists it', async () => {
    stubContexts({
      current: { kubeconfig: '/gone.yaml', name: 'orphan' },
      kubeconfigs: [defaultKubeconfig(['p-mk1'])],
    });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByLabelText('Kubernetes context')).toHaveTextContent('orphan');
  });

  it('shows why a kubeconfig produced no contexts next to the ones that did', async () => {
    stubContexts({
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [
        defaultKubeconfig(['p-mk1']),
        {
          label: '/home/arch/.kube/work.yaml',
          path: '/home/arch/.kube/work.yaml',
          removable: true,
          contexts: [],
          error: 'kubeconfig: stat /home/arch/.kube/work.yaml: no such file or directory',
        },
      ],
    });

    const user = userEvent.setup();
    render(<ContextPicker onSwitched={vi.fn()} />);

    const menu = await openMenu(user);

    expect(within(menu).getByText(/no such file or directory/)).toBeInTheDocument();
    expect(within(menu).getByRole('button', { name: 'p-mk1' })).toBeInTheDocument();
  });

  it('reports a listing the server refused instead of an empty header slot', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'kubeconfig is unreadable' }),
      }),
    );

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/kubeconfig is unreadable/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('recovers from the retry button once the backend answers', async () => {
    const user = userEvent.setup();
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(listOf(['p-mk1', 'p-mk2'], 'p-mk1')),
      });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByRole('button', { name: 'Retry' });

    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(await screen.findByLabelText('Kubernetes context')).toHaveTextContent('p-mk1');
  });

  it('keeps retrying on its own until the backend comes back', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(listOf(['p-mk1'], 'p-mk1')),
      });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByRole('button', { name: 'Retry' });

    expect(await screen.findByLabelText('Kubernetes context')).toBeInTheDocument();
  });

  it('surfaces the reason spinoza reached no cluster', async () => {
    stubContexts({
      current: { kubeconfig: '', name: '' },
      kubeconfigs: [],
      error: 'kubeconfig has no current-context',
    });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/kubeconfig has no current-context/)).toBeInTheDocument();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    const rejectNonError = vi.fn<() => Promise<never>>().mockRejectedValue('nope');
    const fetchMock = vi.fn((_url: string, init?: { method?: string }) => {
      if (init?.method === 'POST') {
        return rejectNonError();
      }
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(listOf(['p-mk1', 'p-mk2'], 'p-mk2')),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);

    await pick(user, 'p-mk1');

    expect(await screen.findByText('opening the cluster failed')).toBeInTheDocument();
  });

  it('drops a listing that lands after unmount', () => {
    const deferred: { settle: () => void } = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            deferred.settle = () => {
              resolve({
                ok: true,
                json: () => Promise.resolve(listOf(['p-mk1'], 'p-mk1')),
              });
            };
          }),
      ),
    );
    const view = render(<ContextPicker onSwitched={vi.fn()} />);

    view.unmount();
    deferred.settle();

    expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
  });

  it('drops a listing failure that lands after unmount', async () => {
    const deferred = {
      fail: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            deferred.fail = () => {
              reject(new Error('too late'));
            };
          }),
      ),
    );
    const view = render(<ContextPicker onSwitched={vi.fn()} />);

    view.unmount();
    deferred.fail();
    await Promise.resolve();

    expect(screen.queryByText(/too late/)).not.toBeInTheDocument();
  });

  it('calls the cluster by the name it was given', async () => {
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk1'));
    act(() => {
      adoptClusters({
        clusters: [
          {
            id: 'https://p-mk1:6443',
            context: 'p-mk1',
            active: true,
            color: 1,
            label: 'client a prod',
            reopen: true,
            protection: 'open',
            reachable: true,
          },
        ],
        remembered: [],
      });
    });

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findAllByText('client a prod')).not.toHaveLength(0);
  });

  it('opens once while the first open is still going', async () => {
    const user = userEvent.setup();
    const posted = vi.fn();
    let release: ((value: unknown) => void) | null = null;
    vi.stubGlobal(
      'fetch',
      vi.fn((_url: string, init?: { method?: string }) => {
        if (init?.method === 'POST') {
          posted();
          return new Promise((resolve) => {
            release = resolve;
          });
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(listOf(['p-mk1', 'p-mk2'], 'p-mk2')),
        });
      }),
    );
    render(<ContextPicker onSwitched={vi.fn()} />);

    await pick(user, 'p-mk1');
    await pick(user, 'p-mk1');

    expect(posted).toHaveBeenCalledTimes(1);
    expect(release).not.toBeNull();
  });

  it('re-picking the context in front brings its tab forward', async () => {
    const user = userEvent.setup();
    const calls = stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));
    render(<ContextPicker onSwitched={vi.fn()} />);

    await pick(user, 'p-mk2');

    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(1);
  });

  it('survives a response with no kubeconfigs field', async () => {
    stubContexts({});

    render(<ContextPicker onSwitched={vi.fn()} />);

    await waitFor(() => {
      expect(screen.queryByLabelText('Kubernetes context')).not.toBeInTheDocument();
    });
  });

  it('names a contexts request that failed outright', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/offline/)).toBeInTheDocument();
  });

  it('falls back to a generic message when the listing rejects with a non-Error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));

    render(<ContextPicker onSwitched={vi.fn()} />);

    expect(await screen.findByText(/the context list could not be loaded/)).toBeInTheDocument();
  });

  it('opens the kubeconfig list from the top bar', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1'], 'p-mk1'));
    render(<ContextPicker onSwitched={vi.fn()} />);

    await pick(user, 'Manage kubeconfigs');

    expect(await screen.findByLabelText('Add a kubeconfig')).toBeInTheDocument();
  });

  it('closes the kubeconfig list again', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1'], 'p-mk1'));
    render(<ContextPicker onSwitched={vi.fn()} />);
    await pick(user, 'Manage kubeconfigs');

    await user.click(screen.getByRole('button', { name: 'Close' }));

    await waitFor(() => {
      expect(screen.getByLabelText('Kubeconfigs')).not.toHaveAttribute('open');
    });
  });

  it('takes the list a kubeconfig change hands back', async () => {
    const user = userEvent.setup();
    const added = {
      label: '/home/arch/.kube/work.yaml',
      path: '/home/arch/.kube/work.yaml',
      removable: true,
      contexts: [{ name: 'staging', cluster: 'work' }],
    };
    const fetchMock = vi.fn((url: string, init?: { method?: string }) => {
      if (url.startsWith('/api/kubeconfigs?')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({
              current: { kubeconfig: '', name: 'p-mk1' },
              kubeconfigs: [defaultKubeconfig(['p-mk1']), added],
            }),
        });
      }
      if (url.startsWith('/api/kubeconfigs/picker')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ available: false }) });
      }
      expect(init?.method).toBeUndefined();
      return Promise.resolve({ ok: true, json: () => Promise.resolve(listOf(['p-mk1'], 'p-mk1')) });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await pick(user, 'Manage kubeconfigs');

    await user.type(screen.getByLabelText('Add a kubeconfig'), '/home/arch/.kube/work.yaml');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    expect(await screen.findByRole('button', { name: 'staging' })).toBeInTheDocument();
  });

  it('notices on its own that a kubeconfig stopped reading', async () => {
    vi.useFakeTimers();
    const healthy = listOf(['p-mk1'], 'p-mk1');
    const broken = {
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [{ ...defaultKubeconfig([]), error: 'no such file or directory' }],
    };
    let answer: unknown = healthy;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(answer) })),
    );
    render(<ContextPicker onSwitched={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(useContextsStore.getState().list.kubeconfigs[0].error).toBeUndefined();

    answer = broken;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(useContextsStore.getState().list.kubeconfigs[0].error).toBe('no such file or directory');
    vi.useRealTimers();
  });

  it('stops refreshing once the token is from an earlier run', async () => {
    vi.useFakeTimers();
    const calls = stubContexts(listOf(['p-mk1'], 'p-mk1'));
    render(<ContextPicker onSwitched={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    const before = calls.length;

    expireSession();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(120000);
    });

    expect(calls).toHaveLength(before);
    vi.useRealTimers();
  });

  it('leaves a refresh alone that fails while the cluster is unreachable', async () => {
    vi.useFakeTimers();
    let failing = false;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        if (failing) {
          return Promise.reject(new Error('offline'));
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(listOf(['p-mk1'], 'p-mk1')),
        });
      }),
    );
    render(<ContextPicker onSwitched={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    failing = true;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(useContextsStore.getState().list.kubeconfigs).toHaveLength(1);
    vi.useRealTimers();
  });

  it('does not overlap context refreshes', async () => {
    vi.useFakeTimers();
    const healthy = listOf(['p-mk1'], 'p-mk1');
    let finishRefresh!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const refresh = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishRefresh = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(healthy) })
      .mockImplementationOnce(() => refresh)
      .mockResolvedValue({ ok: true, json: () => Promise.resolve(healthy) });
    vi.stubGlobal('fetch', fetchMock);
    render(<ContextPicker onSwitched={vi.fn()} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(90000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      finishRefresh({ ok: true, json: () => Promise.resolve(healthy) });
      await refresh;
      await Promise.resolve();
      await vi.advanceTimersByTimeAsync(30000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(3);
  });

  it('offers kubeconfig management inside the dropdown, not beside it', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1'], 'p-mk1'));
    render(<ContextPicker onSwitched={vi.fn()} />);

    await openMenu(user);

    expect(screen.getByRole('button', { name: 'Manage kubeconfigs' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Kubeconfigs' })).not.toBeInTheDocument();
  });

  it('stays on the same context when the management option is picked', async () => {
    const user = userEvent.setup();
    const calls = stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk1'));
    render(<ContextPicker onSwitched={vi.fn()} />);

    await pick(user, 'Manage kubeconfigs');

    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
    expect(await screen.findByLabelText('Kubernetes context')).toHaveTextContent('p-mk1');
  });

  it('still reaches the kubeconfigs when there is no context to pick', async () => {
    const user = userEvent.setup();
    stubContexts({ current: { kubeconfig: '', name: '' }, kubeconfigs: [] });
    render(<ContextPicker onSwitched={vi.fn()} />);

    await user.click(await screen.findByRole('button', { name: 'Kubeconfigs' }));

    expect(await screen.findByLabelText('Add a kubeconfig')).toBeInTheDocument();
  });
});

describe('the context menu going away on its own', () => {
  it('closes when something else on the page is clicked', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));

    render(
      <div>
        <ContextPicker onSwitched={vi.fn()} />
        <button type="button">elsewhere</button>
      </div>,
    );
    const menu = await openMenu(user);
    expect(menu).toHaveAttribute('open');

    await user.click(screen.getByRole('button', { name: 'elsewhere' }));

    expect(menu).not.toHaveAttribute('open');
  });

  it('closes on escape', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));

    render(<ContextPicker onSwitched={vi.fn()} />);
    const menu = await openMenu(user);

    await user.keyboard('{Escape}');

    expect(menu).not.toHaveAttribute('open');
  });

  it('stays open while the pointer is inside it', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));

    render(<ContextPicker onSwitched={vi.fn()} />);
    const menu = await openMenu(user);

    await user.pointer({ target: screen.getByText('p-mk1'), keys: '[MouseLeft>]' });

    expect(menu).toHaveAttribute('open');
  });
});

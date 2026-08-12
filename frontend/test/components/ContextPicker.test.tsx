import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ContextPicker from '../../src/components/ContextPicker';
import { useToastsStore } from '../../src/store/toasts';

function selectedOption(scope: HTMLElement, name: string): HTMLOptionElement {
  const option = within(scope).getByRole('option', { name });
  if (!(option instanceof HTMLOptionElement)) {
    throw new Error(`${name} is not an option`);
  }
  return option;
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
  useToastsStore.getState().clear();
});

describe('ContextPicker', () => {
  it('lists every context with the current one selected', async () => {
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));

    render(<ContextPicker onSwitched={vi.fn()} />);

    const picker = await screen.findByLabelText('Kubernetes context');
    expect(selectedOption(picker, 'p-mk2').selected).toBe(true);
    expect(screen.getByRole('option', { name: 'p-mk1' })).toBeInTheDocument();
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

    render(<ContextPicker onSwitched={vi.fn()} />);

    const picker = await screen.findByLabelText('Kubernetes context');
    const groups = within(picker).getAllByRole('group');
    expect(groups.map((group) => group.getAttribute('label'))).toEqual([
      '/home/arch/.kube/config',
      '/home/arch/.kube/work.yaml',
    ]);
    expect(screen.getByRole('option', { name: 'staging' })).toHaveAttribute(
      'title',
      'cluster work · namespace apps',
    );
  });

  it('switches to the chosen context and tells the app to reconnect', async () => {
    const user = userEvent.setup();
    const calls = stubContexts(
      listOf(['p-mk1', 'p-mk2'], 'p-mk2'),
      listOf(['p-mk1', 'p-mk2'], 'p-mk1'),
    );
    const onSwitched = vi.fn();
    render(<ContextPicker onSwitched={onSwitched} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), '0.0');

    await waitFor(() => {
      expect(onSwitched).toHaveBeenCalled();
    });
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.url).toContain('name=p-mk1');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Switched to p-mk1' }),
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
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), '1.0');

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'POST')).toBe(true);
    });
    const post = calls.find((call) => call.method === 'POST');
    expect(post?.url).toContain('kubeconfig=%2Fhome%2Farch%2F.kube%2Fwork.yaml');
  });

  it('reports a switch the server refused and keeps the old context', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));
    const onSwitched = vi.fn();
    render(<ContextPicker onSwitched={onSwitched} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), '0.0');

    expect(await screen.findByText('context "gone" does not exist')).toBeInTheDocument();
    expect(onSwitched).not.toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'Switching to p-mk1: context "gone" does not exist',
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

    const picker = await screen.findByLabelText('Kubernetes context');
    expect(selectedOption(picker, 'orphan').selected).toBe(true);
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

    render(<ContextPicker onSwitched={vi.fn()} />);

    const picker = await screen.findByLabelText('Kubernetes context');
    expect(within(picker).getByText(/no such file or directory/)).toBeInTheDocument();
    expect(within(picker).getByRole('option', { name: 'p-mk1' })).toBeInTheDocument();
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

    const picker = await screen.findByLabelText('Kubernetes context');
    expect(selectedOption(picker, 'p-mk1').selected).toBe(true);
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
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), '0.0');

    expect(await screen.findByText('switching context failed')).toBeInTheDocument();
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

  it('ignores re-picking the context already in use', async () => {
    const user = userEvent.setup();
    const calls = stubContexts(listOf(['p-mk1', 'p-mk2'], 'p-mk2'));
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByLabelText('Kubernetes context');

    await user.selectOptions(screen.getByLabelText('Kubernetes context'), '0.1');

    expect(calls.filter((call) => call.method === 'POST')).toHaveLength(0);
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
    await screen.findByLabelText('Kubernetes context');

    await user.click(screen.getByRole('button', { name: 'Kubeconfigs' }));

    expect(await screen.findByText('/home/arch/.kube/config')).toBeInTheDocument();
  });

  it('closes the kubeconfig list again', async () => {
    const user = userEvent.setup();
    stubContexts(listOf(['p-mk1'], 'p-mk1'));
    render(<ContextPicker onSwitched={vi.fn()} />);
    await screen.findByLabelText('Kubernetes context');
    await user.click(screen.getByRole('button', { name: 'Kubeconfigs' }));

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
    await screen.findByLabelText('Kubernetes context');
    await user.click(screen.getByRole('button', { name: 'Kubeconfigs' }));

    await user.type(screen.getByLabelText('Add a kubeconfig'), '/home/arch/.kube/work.yaml');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    expect(await screen.findByRole('option', { name: 'staging' })).toBeInTheDocument();
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  addKubeconfig,
  confirmName,
  contextAnnounced,
  contextGroups,
  fetchContexts,
  fetchFilePicker,
  pickKubeconfigFile,
  removeKubeconfig,
  sameContext,
  setProtection,
} from '../../src/lib/contexts';
import type { ContextList } from '../../src/lib/types';
import { EMPTY_CONTEXTS, useContextsStore } from '../../src/store/contexts';
import { bumpClusterEpoch } from '../../src/store/cluster';

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    void url;
    void init;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const list: ContextList = {
  current: { kubeconfig: '', name: 'p-mk1' },
  protection: 'unknown',
  kubeconfigs: [
    {
      label: '/home/arch/.kube/config',
      path: '',
      removable: false,
      contexts: [
        { name: 'p-mk1', cluster: 'p-mk1' },
        { name: 'p-mk2', cluster: 'p-mk2', namespace: 'flux-system' },
      ],
    },
    {
      label: '/home/arch/.kube/work.yaml',
      path: '/home/arch/.kube/work.yaml',
      removable: true,
      contexts: [{ name: 'p-mk1', cluster: 'work' }],
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
  useContextsStore.getState().setList(EMPTY_CONTEXTS);
});

describe('a cluster announcing itself', () => {
  it('reads the list again when the announced context is a different one', async () => {
    stub(list);

    await contextAnnounced('p-mk1');

    expect(useContextsStore.getState().list.current.name).toBe('p-mk1');
  });

  it('leaves the list alone when the backend cannot give it one', async () => {
    useContextsStore.getState().setList(list);
    stub({ message: 'kubeconfig is unreadable' }, false, 500);

    await contextAnnounced('p-mk2');

    expect(useContextsStore.getState().list).toBe(list);
  });

  it('does not overwrite a context list changed while an announcement was loading', async () => {
    let finish!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const response = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finish = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(() => response),
    );
    const loading = contextAnnounced('p-mk1');
    const newer = { ...list, current: { kubeconfig: '', name: 'p-mk2' } };
    useContextsStore.getState().setList(newer);

    finish({ ok: true, json: () => Promise.resolve(list) });
    await loading;

    expect(useContextsStore.getState().list).toBe(newer);
  });

  it('does not apply an announcement from the previous cluster epoch', async () => {
    let finish!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const response = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finish = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn(() => response),
    );
    const loading = contextAnnounced('p-mk1');
    bumpClusterEpoch();

    finish({ ok: true, json: () => Promise.resolve(list) });
    await loading;

    expect(useContextsStore.getState().list).toBe(EMPTY_CONTEXTS);
  });

  it('does not apply an announcement after a newer one starts', async () => {
    let finishFirst!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const first = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishFirst = resolve;
    });
    const newer = { ...list, current: { kubeconfig: '', name: 'p-mk2' } };
    const fetchMock = vi
      .fn()
      .mockImplementationOnce(() => first)
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(newer) });
    vi.stubGlobal('fetch', fetchMock);
    const olderAnnouncement = contextAnnounced('p-mk1');

    await contextAnnounced('p-mk2');
    finishFirst({ ok: true, json: () => Promise.resolve(list) });
    await olderAnnouncement;

    expect(useContextsStore.getState().list.current.name).toBe('p-mk2');
  });
});

describe('contextGroups', () => {
  it('keeps every context under the kubeconfig it came from', () => {
    const groups = contextGroups(list);

    expect(groups.map((group) => group.label)).toEqual([
      '/home/arch/.kube/config',
      '/home/arch/.kube/work.yaml',
    ]);
    expect(groups[1].entries[0].kubeconfig).toBe('/home/arch/.kube/work.yaml');
  });

  it('gives a context in two kubeconfigs two values', () => {
    const values = contextGroups(list)
      .flatMap((group) => group.entries)
      .filter((entry) => entry.name === 'p-mk1')
      .map((entry) => entry.value);

    expect(new Set(values).size).toBe(2);
  });

  it('names the cluster and the namespace a context points at', () => {
    const groups = contextGroups(list);

    expect(groups[0].entries[0].cluster).toBe('cluster p-mk1');
    expect(groups[0].entries[1].cluster).toBe('cluster p-mk2, namespace flux-system');
  });

  it('leaves out a namespace the context does not set', () => {
    const groups = contextGroups({
      current: { kubeconfig: '', name: 'x' },
      protection: 'unknown',
      kubeconfigs: [
        {
          label: 'l',
          path: '',
          removable: false,
          contexts: [{ name: 'x', cluster: 'c', namespace: '' }],
        },
      ],
    });

    expect(groups[0].entries[0].cluster).toBe('cluster c');
  });
});

describe('sameContext', () => {
  it('tells apart the same context name in two kubeconfigs', () => {
    const [fallback, work] = contextGroups(list)
      .flatMap((group) => group.entries)
      .filter((entry) => entry.name === 'p-mk1');

    expect(sameContext(fallback, { kubeconfig: '', name: 'p-mk1' })).toBe(true);
    expect(sameContext(work, { kubeconfig: '', name: 'p-mk1' })).toBe(false);
  });
});

describe('fetchContexts', () => {
  it('fills in what the backend left out', async () => {
    stub({ kubeconfigs: [{}] });

    const found = await fetchContexts();

    expect(found.current).toEqual({ kubeconfig: '', name: '' });
    expect(found.protection).toBe('unknown');
    expect(found.kubeconfigs).toEqual([
      { contexts: [], error: undefined, label: '', path: '', removable: false },
    ]);
  });

  it('reports a listing the backend refused', async () => {
    stub({ message: 'kubeconfig is unreadable' }, false, 500);

    await expect(fetchContexts()).rejects.toThrow('kubeconfig is unreadable');
  });
});

describe('addKubeconfig', () => {
  it('sends the path it was given', async () => {
    const fetchMock = stub(list);

    await addKubeconfig('~/.kube/work.yaml');

    expect(fetchMock.mock.calls[0][0]).toContain('path=%7E%2F.kube%2Fwork.yaml');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' });
  });

  it('reports a kubeconfig the backend refused', async () => {
    stub({ message: 'that file is not a kubeconfig' }, false, 400);

    await expect(addKubeconfig('/tmp/notes.txt')).rejects.toThrow('not a kubeconfig');
  });
});

describe('removeKubeconfig', () => {
  it('deletes the path it was given', async () => {
    const fetchMock = stub(list);

    await removeKubeconfig('/home/arch/.kube/work.yaml');

    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'DELETE' });
  });

  it('reports a removal the backend refused', async () => {
    stub({ message: 'spinoza is connected through that kubeconfig' }, false, 400);

    await expect(removeKubeconfig('/home/arch/.kube/work.yaml')).rejects.toThrow(
      'connected through',
    );
  });
});

describe('setProtection', () => {
  it('says whether the cluster is protected', async () => {
    const fetchMock = stub({ ...list, protection: 'protected' });

    const got = await setProtection(true);

    expect(fetchMock.mock.calls[0][0]).toContain('protected=true');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' });
    expect(got.protection).toBe('protected');
  });

  it('lifts the protection again', async () => {
    const fetchMock = stub({ ...list, protection: 'open' });

    expect((await setProtection(false)).protection).toBe('open');
    expect(fetchMock.mock.calls[0][0]).toContain('protected=false');
  });

  it('treats a verdict it does not know as unknown', async () => {
    stub({ ...list, protection: 'maybe' });

    expect((await setProtection(true)).protection).toBe('unknown');
  });

  it('reports a change the backend refused', async () => {
    stub({ message: 'the file spinoza keeps this in is read-only' }, false, 500);

    await expect(setProtection(true)).rejects.toThrow('read-only');
  });
});

describe('fetchFilePicker', () => {
  it('reads whether a file dialog can be opened', async () => {
    stub({ available: true });

    expect(await fetchFilePicker()).toEqual({ available: true, reason: undefined });
  });

  it('treats an answer without a verdict as no dialog', async () => {
    stub({ reason: 'only the desktop window can open a file dialog' });

    expect(await fetchFilePicker()).toEqual({
      available: false,
      reason: 'only the desktop window can open a file dialog',
    });
  });

  it('reports a support call that failed', async () => {
    stub({ message: 'spinoza needs the token it printed at startup' }, false, 401);

    await expect(fetchFilePicker()).rejects.toThrow('token');
  });
});

describe('pickKubeconfigFile', () => {
  it('returns the file that was chosen', async () => {
    stub({ path: '/home/arch/.kube/work.yaml' });

    expect(await pickKubeconfigFile()).toBe('/home/arch/.kube/work.yaml');
  });

  it('returns nothing when the dialog was cancelled', async () => {
    stub({});

    expect(await pickKubeconfigFile()).toBe('');
  });

  it('reports a dialog that did not open', async () => {
    stub({ message: 'only the desktop window can open a file dialog' }, false, 501);

    await expect(pickKubeconfigFile()).rejects.toThrow('desktop window');
  });
});

describe('the name a protected cluster asks you to type', () => {
  it('is the object name while the cluster is protected', () => {
    expect(confirmName(true, 'web-0')).toBe('web-0');
  });

  it('is nothing at all while it is not', () => {
    expect(confirmName(false, 'web-0')).toBeUndefined();
  });
});

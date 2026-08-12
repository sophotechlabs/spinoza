import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  addKubeconfig,
  contextGroups,
  entryFor,
  everyContext,
  fetchContexts,
  fetchFilePicker,
  pickKubeconfigFile,
  removeKubeconfig,
  sameContext,
  switchContext,
} from '../../src/lib/contexts';
import type { ContextList } from '../../src/lib/types';

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
    const values = everyContext(contextGroups(list))
      .filter((entry) => entry.name === 'p-mk1')
      .map((entry) => entry.value);

    expect(new Set(values).size).toBe(2);
  });

  it('names the cluster and the namespace a context points at', () => {
    const groups = contextGroups(list);

    expect(groups[0].entries[0].cluster).toBe('cluster p-mk1');
    expect(groups[0].entries[1].cluster).toBe('cluster p-mk2 · namespace flux-system');
  });

  it('leaves out a namespace the context does not set', () => {
    const groups = contextGroups({
      current: { kubeconfig: '', name: 'x' },
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
    const [fallback, work] = everyContext(contextGroups(list)).filter(
      (entry) => entry.name === 'p-mk1',
    );

    expect(sameContext(fallback, { kubeconfig: '', name: 'p-mk1' })).toBe(true);
    expect(sameContext(work, { kubeconfig: '', name: 'p-mk1' })).toBe(false);
  });
});

describe('entryFor', () => {
  it('finds the entry the picker selected', () => {
    const groups = contextGroups(list);

    expect(entryFor(groups, '1.0')?.kubeconfig).toBe('/home/arch/.kube/work.yaml');
  });

  it('has nothing for a value no option carries', () => {
    expect(entryFor(contextGroups(list), 'unlisted')).toBeUndefined();
  });
});

describe('fetchContexts', () => {
  it('fills in what the backend left out', async () => {
    stub({ kubeconfigs: [{}] });

    const found = await fetchContexts();

    expect(found.current).toEqual({ kubeconfig: '', name: '' });
    expect(found.kubeconfigs).toEqual([
      { contexts: [], error: undefined, label: '', path: '', removable: false },
    ]);
  });

  it('reports a listing the backend refused', async () => {
    stub({ message: 'kubeconfig is unreadable' }, false, 500);

    await expect(fetchContexts()).rejects.toThrow('kubeconfig is unreadable');
  });
});

describe('switchContext', () => {
  it('names the context and the kubeconfig it came from', async () => {
    const fetchMock = stub(list);

    await switchContext({ cluster: 'work', kubeconfig: '/work.yaml', name: 'beta', value: '1.0' });

    expect(fetchMock.mock.calls[0][0]).toContain('kubeconfig=%2Fwork.yaml');
    expect(fetchMock.mock.calls[0][0]).toContain('name=beta');
  });

  it('reports a switch the backend refused', async () => {
    stub({ message: 'context "gone" does not exist' }, false, 400);

    await expect(
      switchContext({ cluster: 'c', kubeconfig: '', name: 'gone', value: '0.0' }),
    ).rejects.toThrow('does not exist');
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

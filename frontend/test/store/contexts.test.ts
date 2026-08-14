import { describe, expect, it } from 'vitest';
import type { ContextList } from '../../src/lib/types';
import { EMPTY_CONTEXTS, unreadableCurrent, useContextsStore } from '../../src/store/contexts';

function listWith(error: string | undefined, path: string): ContextList {
  return {
    current: { kubeconfig: path, name: 'p-mk1' },
    kubeconfigs: [
      { label: '/home/arch/.kube/config', path: '', removable: false, contexts: [] },
      { label: '/tmp/work.yaml', path: '/tmp/work.yaml', removable: true, contexts: [], error },
    ],
  };
}

describe('the kubeconfig behind the current context', () => {
  it('is nothing to report while it reads fine', () => {
    expect(unreadableCurrent(listWith(undefined, '/tmp/work.yaml'))).toBeNull();
  });

  it('is reported when the file it came from stopped reading', () => {
    const gone = unreadableCurrent(listWith('no such file or directory', '/tmp/work.yaml'));

    expect(gone?.label).toBe('/tmp/work.yaml');
    expect(gone?.error).toBe('no such file or directory');
  });

  it('ignores a broken file the current context did not come from', () => {
    expect(unreadableCurrent(listWith('no such file or directory', ''))).toBeNull();
  });

  it('says nothing when no cluster is connected at all', () => {
    expect(unreadableCurrent(EMPTY_CONTEXTS)).toBeNull();
  });

  it('covers the default kubeconfig too, which carries no path', () => {
    const list: ContextList = {
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [
        {
          label: '/home/arch/.kube/config',
          path: '',
          removable: false,
          contexts: [],
          error: 'gone',
        },
      ],
    };

    expect(unreadableCurrent(list)?.label).toBe('/home/arch/.kube/config');
  });

  it('is held in a store the whole app can read', () => {
    useContextsStore.getState().setList(listWith('gone', '/tmp/work.yaml'));

    expect(useContextsStore.getState().list.current.name).toBe('p-mk1');

    useContextsStore.getState().reset();

    expect(useContextsStore.getState().list).toEqual(EMPTY_CONTEXTS);
  });
});

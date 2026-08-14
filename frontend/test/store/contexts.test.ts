import { describe, expect, it } from 'vitest';
import { renderHook } from '@testing-library/react';
import type { ContextList, Protection } from '../../src/lib/types';
import {
  EMPTY_CONTEXTS,
  unreadableCurrent,
  useContextsStore,
  useProtectedCluster,
} from '../../src/store/contexts';

function listWith(error: string | undefined, path: string): ContextList {
  return {
    current: { kubeconfig: path, name: 'p-mk1' },
    protection: 'unknown',
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
      protection: 'unknown',
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

describe('whether the current cluster is protected', () => {
  function withProtection(protection: Protection): boolean {
    const list = listWith(undefined, '');
    useContextsStore.getState().setList({ ...list, protection });
    return renderHook(() => useProtectedCluster()).result.current;
  }

  it('is true only once the cluster has been marked', () => {
    expect(withProtection('protected')).toBe(true);
    expect(withProtection('open')).toBe(false);
    expect(withProtection('unknown')).toBe(false);
  });
});

import { beforeEach, describe, expect, it } from 'vitest';
import type { ObjectRef } from '../../src/lib/types';
import {
  MAX_RECENTS,
  clearRecents,
  rememberObject,
  useRecentsStore,
} from '../../src/store/recents';

function ref(name: string, namespace = 'prod'): ObjectRef {
  return { group: '', version: 'v1', resource: 'pods', namespace, name };
}

describe('recent objects', () => {
  beforeEach(() => {
    clearRecents();
  });

  it('starts empty', () => {
    expect(useRecentsStore.getState().recents).toEqual([]);
  });

  it('puts the newest first', () => {
    rememberObject(ref('a'));
    rememberObject(ref('b'));

    expect(useRecentsStore.getState().recents.map((one) => one.name)).toEqual(['b', 'a']);
  });

  it('moves a repeat visit back to the front instead of duplicating it', () => {
    rememberObject(ref('a'));
    rememberObject(ref('b'));
    rememberObject(ref('a'));

    expect(useRecentsStore.getState().recents.map((one) => one.name)).toEqual(['a', 'b']);
  });

  it('tells two same-named objects in different namespaces apart', () => {
    rememberObject(ref('web', 'prod'));
    rememberObject(ref('web', 'staging'));

    expect(useRecentsStore.getState().recents).toHaveLength(2);
  });

  it('keeps only the most recent handful', () => {
    for (let index = 0; index < MAX_RECENTS + 5; index += 1) {
      rememberObject(ref(`pod-${String(index)}`));
    }

    const kept = useRecentsStore.getState().recents;
    expect(kept).toHaveLength(MAX_RECENTS);
    expect(kept[0].name).toBe(`pod-${String(MAX_RECENTS + 4)}`);
  });

  it('empties when the cluster changes', () => {
    rememberObject(ref('a'));

    clearRecents();

    expect(useRecentsStore.getState().recents).toEqual([]);
  });
});

import { beforeEach, describe, expect, it } from 'vitest';
import type { ObjectRef } from '../../src/lib/types';
import {
  MAX_RECENTS,
  clearRecents,
  forgetRecents,
  rememberObject,
  useRecentsStore,
} from '../../src/store/recents';
import { MK1, MK2, showing } from '../helpers-clusters';

function ref(name: string, namespace = 'prod'): ObjectRef {
  return { group: '', version: 'v1', resource: 'pods', namespace, name };
}

function names(cluster: string): string[] {
  return (useRecentsStore.getState().byCluster[cluster] ?? []).map((one) => one.name);
}

describe('recent objects', () => {
  beforeEach(() => {
    clearRecents();
    showing(MK1);
  });

  it('starts empty', () => {
    expect(names(MK1)).toEqual([]);
  });

  it('puts the newest first', () => {
    rememberObject(ref('a'));
    rememberObject(ref('b'));

    expect(names(MK1)).toEqual(['b', 'a']);
  });

  it('moves a repeat visit back to the front instead of duplicating it', () => {
    rememberObject(ref('a'));
    rememberObject(ref('b'));
    rememberObject(ref('a'));

    expect(names(MK1)).toEqual(['a', 'b']);
  });

  it('tells two same-named objects in different namespaces apart', () => {
    rememberObject(ref('web', 'prod'));
    rememberObject(ref('web', 'staging'));

    expect(names(MK1)).toHaveLength(2);
  });

  it('keeps only the most recent handful', () => {
    for (let index = 0; index < MAX_RECENTS + 5; index += 1) {
      rememberObject(ref(`pod-${String(index)}`));
    }

    const kept = names(MK1);
    expect(kept).toHaveLength(MAX_RECENTS);
    expect(kept[0]).toBe(`pod-${String(MAX_RECENTS + 4)}`);
  });

  it("keeps one tab out of another tab's list", () => {
    rememberObject(ref('a'));

    showing(MK2);
    rememberObject(ref('b'));

    expect(names(MK1)).toEqual(['a']);
    expect(names(MK2)).toEqual(['b']);
  });

  it("lets go of a closed tab's list", () => {
    rememberObject(ref('a'));
    showing(MK2);
    rememberObject(ref('b'));

    forgetRecents(MK1);

    expect(names(MK1)).toEqual([]);
    expect(names(MK2)).toEqual(['b']);
  });

  it('empties every tab when the window is torn down', () => {
    rememberObject(ref('a'));

    clearRecents();

    expect(useRecentsStore.getState().byCluster).toEqual({});
  });
});

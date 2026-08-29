import { describe, expect, it } from 'vitest';
import { drop, held, put } from '../../src/store/perCluster';
import { MK1, MK2 } from '../helpers-clusters';

describe('state that belongs to one cluster', () => {
  it('hands back the fallback for a cluster nothing is known about', () => {
    expect(held<number>({}, MK1, 7)).toBe(7);
  });

  it('hands back what was put there', () => {
    expect(held(put<number>({}, MK1, 1), MK1, 7)).toBe(1);
  });

  it('keeps one cluster out of another', () => {
    const both = put(put<number>({}, MK1, 1), MK2, 2);

    expect(held(both, MK1, 0)).toBe(1);
    expect(held(both, MK2, 0)).toBe(2);
  });

  it('leaves the other clusters alone when one is dropped', () => {
    const both = put(put<number>({}, MK1, 1), MK2, 2);

    const left = drop(both, MK1);

    expect(held(left, MK1, 0)).toBe(0);
    expect(held(left, MK2, 0)).toBe(2);
  });

  it('dropping a cluster nothing is known about changes nothing', () => {
    const one = put<number>({}, MK1, 1);

    expect(drop(one, MK2)).toEqual(one);
  });
});

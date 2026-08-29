import { describe, expect, it } from 'vitest';
import { CLUSTER_COLORS, colorNames, colorVar, held } from '../../src/lib/clusterColor';

describe('the colour a tab wears', () => {
  it('offers one for every tab the strip is likely to hold', () => {
    expect(colorNames()).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
    expect(colorNames()).toHaveLength(CLUSTER_COLORS);
  });

  it('asks the stylesheet for the one it was given', () => {
    expect(colorVar(3)).toBe('var(--cluster-3)');
    expect(colorVar(CLUSTER_COLORS)).toBe(`var(--cluster-${String(CLUSTER_COLORS)})`);
  });

  it('falls back to the first for a cluster the server never coloured', () => {
    expect(held(0)).toBe(1);
    expect(colorVar(0)).toBe('var(--cluster-1)');
  });

  it('falls back to the first for anything past the palette', () => {
    expect(held(CLUSTER_COLORS + 1)).toBe(1);
    expect(colorVar(99)).toBe('var(--cluster-1)');
  });
});

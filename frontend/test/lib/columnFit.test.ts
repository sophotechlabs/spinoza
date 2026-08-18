import { describe, expect, it } from 'vitest';
import { extraWidths, widthOf } from '../../src/lib/columnFit';
import type { FitColumn } from '../../src/lib/columnFit';

function column(id: string, size: number, over: Partial<FitColumn> = {}): FitColumn {
  return { id, size, fixed: false, sized: false, ...over };
}

describe('filling the width a table is given', () => {
  it('leaves the columns alone when there is no room to spare', () => {
    expect(extraWidths([column('name', 240)], 0)).toEqual({});
    expect(extraWidths([column('name', 240)], -80)).toEqual({});
  });

  it('shares the slack in proportion to how wide each column already is', () => {
    const stretch = extraWidths([column('name', 300), column('age', 100)], 400);

    expect(stretch).toEqual({ name: 300, age: 100 });
  });

  it('gives the whole slack away, remainder included', () => {
    const stretch = extraWidths([column('a', 100), column('b', 100), column('c', 100)], 100);
    let given = 0;
    for (const extra of Object.values(stretch)) {
      given += extra ?? 0;
    }

    expect(given).toBe(100);
  });

  it('never widens a column that cannot be resized', () => {
    const stretch = extraWidths([column('select', 32, { fixed: true }), column('name', 200)], 100);

    expect(stretch.select).toBeUndefined();
    expect(stretch.name).toBe(100);
  });

  it('spares the columns the user sized while any others can grow', () => {
    const stretch = extraWidths([column('name', 200, { sized: true }), column('age', 100)], 60);

    expect(stretch.name).toBeUndefined();
    expect(stretch.age).toBe(60);
  });

  it('still fills the row when the user has sized every column', () => {
    const stretch = extraWidths(
      [column('name', 300, { sized: true }), column('age', 100, { sized: true })],
      400,
    );

    expect(stretch).toEqual({ name: 300, age: 100 });
  });

  it('has nowhere to put the slack when every column is fixed', () => {
    expect(extraWidths([column('select', 32, { fixed: true })], 200)).toEqual({});
  });

  it('has nothing to share out among zero-width columns', () => {
    expect(extraWidths([column('name', 0)], 200)).toEqual({});
  });

  it('adds a column its share, or nothing when it has none', () => {
    expect(widthOf('name', 240, { name: 60 })).toBe(300);
    expect(widthOf('age', 72, { name: 60 })).toBe(72);
  });
});

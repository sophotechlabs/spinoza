import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import {
  DEFAULT_PLACEMENT,
  PANEL_ORDER,
  PLACEMENT_KEY,
  panelsOn,
  parsePlacement,
  readPlacement,
  usePlacement,
  writePlacement,
} from '../../src/lib/panels';

afterEach(() => {
  window.localStorage.clear();
  vi.restoreAllMocks();
});

describe('parsePlacement', () => {
  it('falls back to the defaults with nothing stored', () => {
    expect(parsePlacement(null)).toEqual(DEFAULT_PLACEMENT);
  });

  it('falls back to the defaults for unreadable json', () => {
    expect(parsePlacement('{not json')).toEqual(DEFAULT_PLACEMENT);
  });

  it('falls back to the defaults for a json value that is not an object', () => {
    expect(parsePlacement('7')).toEqual(DEFAULT_PLACEMENT);
  });

  it('falls back to the defaults for a stored null', () => {
    expect(parsePlacement('null')).toEqual(DEFAULT_PLACEMENT);
  });

  it('keeps the stored side for a known panel', () => {
    const placement = parsePlacement(JSON.stringify({ logs: 'bottom' }));

    expect(placement.logs).toBe('bottom');
    expect(placement.yaml).toBe(DEFAULT_PLACEMENT.yaml);
  });

  it('ignores a panel it does not know', () => {
    const placement = parsePlacement(JSON.stringify({ imaginary: 'left' }));

    expect(placement).toEqual(DEFAULT_PLACEMENT);
  });

  it('ignores a side it does not know', () => {
    const placement = parsePlacement(JSON.stringify({ logs: 'ceiling' }));

    expect(placement.logs).toBe(DEFAULT_PLACEMENT.logs);
  });
});

describe('panelsOn', () => {
  it('lists the panels of one dock in registry order', () => {
    expect(panelsOn(DEFAULT_PLACEMENT, 'bottom')).toEqual(['forwards', 'terminal']);
    expect(panelsOn(DEFAULT_PLACEMENT, 'left')).toEqual([]);
  });

  it('covers every panel across the three docks', () => {
    const spread = [
      ...panelsOn(DEFAULT_PLACEMENT, 'left'),
      ...panelsOn(DEFAULT_PLACEMENT, 'right'),
      ...panelsOn(DEFAULT_PLACEMENT, 'bottom'),
    ];

    expect(spread.sort()).toEqual([...PANEL_ORDER].sort());
  });
});

describe('stored placement', () => {
  it('survives a round trip', () => {
    writePlacement({ ...DEFAULT_PLACEMENT, metrics: 'left' });

    expect(readPlacement().metrics).toBe('left');
  });

  it('falls back to the defaults when storage refuses to read', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });

    expect(readPlacement()).toEqual(DEFAULT_PLACEMENT);
  });

  it('carries on when storage refuses to write', () => {
    vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });

    expect(() => {
      writePlacement(DEFAULT_PLACEMENT);
    }).not.toThrow();
  });
});

describe('usePlacement', () => {
  it('moves a panel and remembers it for the next session', () => {
    const { result } = renderHook(() => usePlacement());

    act(() => {
      result.current.move('logs', 'bottom');
    });

    expect(result.current.placement.logs).toBe('bottom');
    expect(window.localStorage.getItem(PLACEMENT_KEY)).toContain('"logs":"bottom"');
  });

  it('leaves the placement alone when the panel is already there', () => {
    const { result } = renderHook(() => usePlacement());
    const before = result.current.placement;

    act(() => {
      result.current.move('logs', DEFAULT_PLACEMENT.logs);
    });

    expect(result.current.placement).toBe(before);
    expect(window.localStorage.getItem(PLACEMENT_KEY)).toBeNull();
  });

  it('starts from what the last session stored', () => {
    window.localStorage.setItem(PLACEMENT_KEY, JSON.stringify({ terminal: 'left' }));

    const { result } = renderHook(() => usePlacement());

    expect(result.current.placement.terminal).toBe('left');
  });
});

describe('a stored side that is not a string', () => {
  it('is ignored', () => {
    const placement = parsePlacement(JSON.stringify({ logs: 7 }));

    expect(placement.logs).toBe(DEFAULT_PLACEMENT.logs);
  });
});

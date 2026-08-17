import { resetStored } from '../../src/lib/persist';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { PanelContext } from '../../src/lib/panels';
import {
  DEFAULT_LAYOUT,
  DEFAULT_PLACEMENT,
  PANELS,
  PANEL_ORDER,
  panelById,
  panelsOn,
  parseLayout,
  parsePlacement,
  readLayout,
  readPlacement,
  writeLayout,
  writePlacement,
} from '../../src/lib/panels';
import type { ObjectDetail } from '../../src/lib/types';

afterEach(() => {
  resetStored();
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

describe('a stored side that is not a string', () => {
  it('is ignored', () => {
    const placement = parsePlacement(JSON.stringify({ logs: 7 }));

    expect(placement.logs).toBe(DEFAULT_PLACEMENT.logs);
  });
});

describe('the panel registry', () => {
  it('is the single source of the order, the labels and the default docks', () => {
    expect(PANEL_ORDER).toEqual(PANELS.map((panel) => panel.id));
    for (const panel of PANELS) {
      expect(panelById(panel.id)).toBe(panel);
      expect(DEFAULT_PLACEMENT[panel.id]).toBe(panel.defaultSide);
      expect(panel.label).not.toBe('');
      expect(panel.hint).not.toBe('');
    }
  });

  function ctx(overrides: Partial<PanelContext> = {}): PanelContext {
    return { selection: null, detail: null, pod: null, ...overrides };
  }

  const podRef = { group: '', version: 'v1', resource: 'pods', namespace: 'prod', name: 'web' };
  const podSelection = { ref: podRef, row: null };
  const podDetail: ObjectDetail = {
    apiVersion: 'v1',
    kind: 'Pod',
    name: 'web',
    namespace: 'prod',
    uid: 'uid-web',
    createdAt: '2026-08-03T09:00:00Z',
    yaml: '',
  };

  it('opens the object panels only once something is selected', () => {
    expect(panelById('overview').enabled(ctx())).toBe(false);
    expect(panelById('yaml').enabled(ctx({ selection: podSelection }))).toBe(true);
    expect(panelById('events').enabled(ctx({ selection: podSelection }))).toBe(true);
  });

  it('keeps logs and metrics for pods', () => {
    expect(panelById('logs').enabled(ctx({ selection: podSelection }))).toBe(false);
    expect(panelById('logs').enabled(ctx({ selection: podSelection, detail: podDetail }))).toBe(
      true,
    );
    expect(
      panelById('metrics').enabled(ctx({ detail: { ...podDetail, kind: 'Deployment' } })),
    ).toBe(false);
  });

  it('keeps the terminal open at all times, pod selected or not', () => {
    expect(panelById('terminal').enabled(ctx())).toBe(true);
    expect(
      panelById('terminal').enabled(
        ctx({ pod: { namespace: 'prod', name: 'web', containers: ['app'] } }),
      ),
    ).toBe(true);
  });

  it('keeps forwards open at all times', () => {
    expect(panelById('forwards').enabled(ctx())).toBe(true);
  });
});

describe('parseLayout', () => {
  it('falls back to the defaults with nothing stored', () => {
    expect(parseLayout(null)).toEqual(DEFAULT_LAYOUT);
  });

  it('falls back to the defaults for unreadable json', () => {
    expect(parseLayout('{not json')).toEqual(DEFAULT_LAYOUT);
  });

  it('falls back to the defaults for a json value that is not an object', () => {
    expect(parseLayout('7')).toEqual(DEFAULT_LAYOUT);
  });

  it('falls back to the defaults for a stored null', () => {
    expect(parseLayout('null')).toEqual(DEFAULT_LAYOUT);
  });

  it('keeps a stored dock size and the sidebar width', () => {
    const layout = parseLayout(JSON.stringify({ sizes: { right: 700 }, sidebar: 300 }));

    expect(layout.sizes.right).toBe(700);
    expect(layout.sizes.left).toBeNull();
    expect(layout.sidebar).toBe(300);
  });

  it('drops a size that is not a usable number', () => {
    const layout = parseLayout(JSON.stringify({ sizes: { right: 'wide', bottom: null } }));

    expect(layout.sizes.right).toBeNull();
    expect(layout.sizes.bottom).toBeNull();
  });

  it('drops a size that is not finite', () => {
    expect(parseLayout('{"sidebar":1e999}').sidebar).toBeNull();
  });

  it('reads collapsed docks as booleans only', () => {
    const layout = parseLayout(JSON.stringify({ collapsed: { left: true, right: 'yes' } }));

    expect(layout.collapsed.left).toBe(true);
    expect(layout.collapsed.right).toBe(false);
  });

  it('keeps an open tab it recognises and drops one it does not', () => {
    const layout = parseLayout(JSON.stringify({ active: { right: 'yaml', bottom: 'imaginary' } }));

    expect(layout.active.right).toBe('yaml');
    expect(layout.active.bottom).toBeNull();
  });

  it('ignores a section that is not an object', () => {
    const layout = parseLayout(JSON.stringify({ sizes: 7, collapsed: null }));

    expect(layout.sizes.right).toBeNull();
    expect(layout.collapsed.right).toBe(false);
  });
});

describe('stored layout', () => {
  it('survives a round trip', () => {
    writeLayout({
      sizes: { left: null, right: 640, bottom: null },
      collapsed: { left: false, right: false, bottom: true },
      active: { left: null, right: 'events', bottom: null },
      sidebar: 300,
    });

    const layout = readLayout();
    expect(layout.sizes.right).toBe(640);
    expect(layout.collapsed.bottom).toBe(true);
    expect(layout.active.right).toBe('events');
    expect(layout.sidebar).toBe(300);
  });

  it('falls back to the defaults when storage refuses to read', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });

    expect(readLayout()).toEqual(DEFAULT_LAYOUT);
  });

  it('carries on when storage refuses to write', () => {
    vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota');
    });

    expect(() => {
      writeLayout(DEFAULT_LAYOUT);
    }).not.toThrow();
  });
});

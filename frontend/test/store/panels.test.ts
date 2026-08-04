import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DEFAULT_PLACEMENT, PLACEMENT_KEY } from '../../src/lib/panels';

async function freshStore() {
  vi.resetModules();
  return import('../../src/store/panels');
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  window.localStorage.clear();
});

describe('where each panel is docked', () => {
  it('starts from the defaults when nothing was stored', async () => {
    const { usePanelsStore } = await freshStore();

    expect(usePanelsStore.getState().placement).toEqual(DEFAULT_PLACEMENT);
  });

  it('starts from what the last session stored', async () => {
    window.localStorage.setItem(PLACEMENT_KEY, JSON.stringify({ terminal: 'left' }));
    const { usePanelsStore } = await freshStore();

    expect(usePanelsStore.getState().placement.terminal).toBe('left');
  });

  it('moves a panel and remembers it', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().move('logs', 'left');

    expect(usePanelsStore.getState().placement.logs).toBe('left');
    expect(window.localStorage.getItem(PLACEMENT_KEY)).toContain('"logs":"left"');
  });

  it('does nothing when the panel is already on that side', async () => {
    const { usePanelsStore } = await freshStore();
    const before = usePanelsStore.getState().placement;

    usePanelsStore.getState().move('logs', DEFAULT_PLACEMENT.logs);

    expect(usePanelsStore.getState().placement).toBe(before);
    expect(window.localStorage.getItem(PLACEMENT_KEY)).toBeNull();
  });

  it('puts everything back where it started on reset', async () => {
    const { usePanelsStore } = await freshStore();
    usePanelsStore.getState().move('logs', 'left');

    usePanelsStore.getState().reset();

    expect(usePanelsStore.getState().placement).toEqual(DEFAULT_PLACEMENT);
    expect(window.localStorage.getItem(PLACEMENT_KEY)).toContain(
      `"logs":"${DEFAULT_PLACEMENT.logs}"`,
    );
  });
});

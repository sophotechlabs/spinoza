import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { readStored, resetStored, writeStored } from '../../src/lib/persist';
import { DEFAULT_PLACEMENT, PLACEMENT_KEY } from '../../src/lib/panels';

async function freshStore() {
  vi.resetModules();
  return import('../../src/store/panels');
}

beforeEach(() => {
  resetStored();
});

afterEach(() => {
  resetStored();
});

describe('where each panel is docked', () => {
  it('starts from the defaults when nothing was stored', async () => {
    const { usePanelsStore } = await freshStore();

    expect(usePanelsStore.getState().placement).toEqual(DEFAULT_PLACEMENT);
  });

  it('starts from what the last session stored', async () => {
    writeStored(PLACEMENT_KEY, JSON.stringify({ terminal: 'left' }));
    const { usePanelsStore } = await freshStore();

    expect(usePanelsStore.getState().placement.terminal).toBe('left');
  });

  it('moves a panel and remembers it', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().move('logs', 'left');

    expect(usePanelsStore.getState().placement.logs).toBe('left');
    expect(readStored(PLACEMENT_KEY)).toContain('"logs":"left"');
  });

  it('does nothing when the panel is already on that side', async () => {
    const { usePanelsStore } = await freshStore();
    const before = usePanelsStore.getState().placement;

    usePanelsStore.getState().move('logs', DEFAULT_PLACEMENT.logs);

    expect(usePanelsStore.getState().placement).toBe(before);
    expect(readStored(PLACEMENT_KEY)).toBeNull();
  });

  it('puts everything back where it started on reset', async () => {
    const { usePanelsStore } = await freshStore();
    usePanelsStore.getState().move('logs', 'left');

    usePanelsStore.getState().reset();

    expect(usePanelsStore.getState().placement).toEqual(DEFAULT_PLACEMENT);
    expect(readStored(PLACEMENT_KEY)).toContain(`"logs":"${DEFAULT_PLACEMENT.logs}"`);
  });
});

describe('the rest of the layout', () => {
  it('starts unset so each dock uses its own default', async () => {
    const { usePanelsStore } = await freshStore();

    expect(usePanelsStore.getState().sizes).toEqual({ left: null, right: null, bottom: null });
    expect(usePanelsStore.getState().sidebar).toBeNull();
    expect(usePanelsStore.getState().collapsed.right).toBe(false);
    expect(usePanelsStore.getState().active.right).toBeNull();
  });

  it('remembers a dock size for the next session', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().resize('right', 700);

    expect(usePanelsStore.getState().sizes.right).toBe(700);
    const reopened = await freshStore();
    expect(reopened.usePanelsStore.getState().sizes.right).toBe(700);
  });

  it('remembers the sidebar width', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().resizeSidebar(300);

    const reopened = await freshStore();
    expect(reopened.usePanelsStore.getState().sidebar).toBe(300);
  });

  it('remembers a collapsed dock', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().collapse('bottom', true);

    const reopened = await freshStore();
    expect(reopened.usePanelsStore.getState().collapsed.bottom).toBe(true);
  });

  it('remembers the open tab of each dock', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().activate('right', 'events');

    const reopened = await freshStore();
    expect(reopened.usePanelsStore.getState().active.right).toBe('events');
  });

  it('puts sizes, collapse and open tabs back on reset', async () => {
    const { usePanelsStore } = await freshStore();
    usePanelsStore.getState().resize('right', 700);
    usePanelsStore.getState().resizeSidebar(300);
    usePanelsStore.getState().collapse('bottom', true);
    usePanelsStore.getState().activate('right', 'events');

    usePanelsStore.getState().reset();

    expect(usePanelsStore.getState().sizes).toEqual({ left: null, right: null, bottom: null });
    expect(usePanelsStore.getState().sidebar).toBeNull();
    expect(usePanelsStore.getState().collapsed.bottom).toBe(false);
    expect(usePanelsStore.getState().active.right).toBeNull();
    const reopened = await freshStore();
    expect(reopened.usePanelsStore.getState().sizes.right).toBeNull();
  });

  it('keeps each stored piece independent of the others', async () => {
    const { usePanelsStore } = await freshStore();

    usePanelsStore.getState().resize('right', 700);
    usePanelsStore.getState().collapse('left', true);

    const reopened = await freshStore();
    expect(reopened.usePanelsStore.getState().sizes.right).toBe(700);
    expect(reopened.usePanelsStore.getState().collapsed.left).toBe(true);
  });
});

describe('showing the details again', () => {
  it('opens the dock the overview sits in', async () => {
    const { usePanelsStore, revealDetails } = await freshStore();
    const side = usePanelsStore.getState().placement.overview;
    usePanelsStore.getState().collapse(side, true);

    revealDetails();

    expect(usePanelsStore.getState().collapsed[side]).toBe(false);
  });

  it('follows the overview to whichever dock it was moved to', async () => {
    const { usePanelsStore, revealDetails } = await freshStore();
    usePanelsStore.getState().move('overview', 'bottom');
    usePanelsStore.getState().collapse('bottom', true);
    usePanelsStore.getState().collapse('right', true);

    revealDetails();

    expect(usePanelsStore.getState().collapsed.bottom).toBe(false);
    expect(usePanelsStore.getState().collapsed.right).toBe(true);
  });

  it('leaves an open dock alone', async () => {
    const { usePanelsStore, revealDetails } = await freshStore();
    const side = usePanelsStore.getState().placement.overview;
    usePanelsStore.getState().resize(side, 700);

    revealDetails();

    expect(usePanelsStore.getState().collapsed[side]).toBe(false);
    expect(usePanelsStore.getState().sizes[side]).toBe(700);
  });
});

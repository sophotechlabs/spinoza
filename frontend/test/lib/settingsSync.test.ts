import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { catchUp, watchSettings } from '../../src/lib/settingsSync';
import {
  SAVE_DELAY_MS,
  flush,
  hydrate,
  isSaving,
  readStored,
  resetStored,
  startSaving,
  stopSaving,
  storedKeys,
  withoutTrackingChanges,
  writeStored,
} from '../../src/lib/persist';
import { useThemeStore } from '../../src/store/theme';
import { useSettingsStore } from '../../src/store/settings';
import { THEME_KEY } from '../../src/lib/theme';
import { SETTINGS_KEY } from '../../src/lib/settings';

function served(values: Record<string, string>) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: () => Promise.resolve({ values }),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function held(): Record<string, string> {
  const out: Record<string, string> = {};
  for (const key of storedKeys()) {
    const value = readStored(key);
    if (value !== null) {
      out[key] = value;
    }
  }
  return out;
}

function loadedWith(values: Record<string, string>) {
  window.__SPINOZA_SETTINGS__ = JSON.stringify(values);
  hydrate();
  withoutTrackingChanges(() => {
    useThemeStore.getState().adoptStored();
    useSettingsStore.getState().adoptStored();
  });
}

beforeEach(() => {
  resetStored();
  stopSaving();
  window.localStorage.clear();
  loadedWith({ [THEME_KEY]: 'nord' });
});

afterEach(() => {
  vi.unstubAllGlobals();
  delete window.__SPINOZA_SETTINGS__;
  resetStored();
});

describe('catchUp', () => {
  it('takes the theme another window chose', async () => {
    expect(useThemeStore.getState().preference).toBe('nord');
    served({ [THEME_KEY]: 'borg' });

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('borg');
    expect(readStored(THEME_KEY)).toBe('borg');
  });

  it('does not touch the theme when nothing changed', async () => {
    served(held());
    const adopt = vi.spyOn(useThemeStore.getState(), 'adoptStored');

    await catchUp();

    expect(adopt).not.toHaveBeenCalled();
    adopt.mockRestore();
  });

  it('takes a change that keeps the number of settings the same', async () => {
    const changed = held();
    changed[THEME_KEY] = 'borg';
    served(changed);

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('borg');
  });

  it('takes a setting it has never seen', async () => {
    const grown = held();
    grown['spinoza.sidebar.v1'] = '{"Cluster":true}';
    served(grown);

    await catchUp();

    expect(readStored('spinoza.sidebar.v1')).toBe('{"Cluster":true}');
  });

  it('keeps what it has when the server cannot be reached', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('nord');
  });

  it('keeps what it has when the server refuses', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500 }));

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('nord');
  });

  it('keeps what it has when the answer is not settings', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve('nope') }),
    );

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('nord');
  });

  it('keeps what it has when the answer carries no values', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ values: null }) }),
    );

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('nord');
  });

  it('takes only the strings out of the answer', async () => {
    served({ [THEME_KEY]: 'borg' });
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ values: { [THEME_KEY]: 'borg', junk: 7 } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await catchUp();

    expect(useThemeStore.getState().preference).toBe('borg');
    expect(readStored('junk')).toBeNull();
  });

  it('does not save what it adopted', async () => {
    vi.useFakeTimers();
    startSaving();
    const fetchMock = served({ [THEME_KEY]: 'borg' });

    await catchUp();
    await vi.advanceTimersByTimeAsync(SAVE_DELAY_MS * 3);

    const puts = fetchMock.mock.calls.filter(
      (call) => (call[1] as RequestInit | undefined)?.method === 'PUT',
    );
    expect(puts).toHaveLength(0);
    expect(isSaving()).toBe(true);
    vi.useRealTimers();
  });
});

describe('watchSettings', () => {
  it('catches up when the window comes back', async () => {
    const stop = watchSettings();
    served({ [THEME_KEY]: 'borg' });

    window.dispatchEvent(new Event('focus'));

    await vi.waitFor(() => {
      expect(useThemeStore.getState().preference).toBe('borg');
    });
    stop();
  });

  it('catches up when the tab becomes visible again', async () => {
    const stop = watchSettings();
    served({ [THEME_KEY]: 'borg' });

    document.dispatchEvent(new Event('visibilitychange'));

    await vi.waitFor(() => {
      expect(useThemeStore.getState().preference).toBe('borg');
    });
    stop();
  });

  it('coalesces the focus and visibility events from one return', async () => {
    const stop = watchSettings();
    let finish!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const response = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finish = resolve;
    });
    const fetchMock = vi.fn(() => response);
    vi.stubGlobal('fetch', fetchMock);

    window.dispatchEvent(new Event('focus'));
    document.dispatchEvent(new Event('visibilitychange'));

    expect(fetchMock).toHaveBeenCalledTimes(1);
    finish({ ok: true, json: () => Promise.resolve({ values: { [THEME_KEY]: 'borg' } }) });
    await response;
    await vi.waitFor(() => {
      expect(useThemeStore.getState().preference).toBe('borg');
    });
    stop();
  });

  it('does nothing while the tab is being hidden', async () => {
    const stop = watchSettings();
    const fetchMock = served({ [THEME_KEY]: 'borg' });
    const hidden = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');

    document.dispatchEvent(new Event('visibilitychange'));
    await Promise.resolve();

    expect(fetchMock).not.toHaveBeenCalled();
    hidden.mockRestore();
    stop();
  });

  it('stops listening once it is torn down', async () => {
    const stop = watchSettings();
    stop();
    const fetchMock = served({ [THEME_KEY]: 'borg' });

    window.dispatchEvent(new Event('focus'));
    document.dispatchEvent(new Event('visibilitychange'));
    await Promise.resolve();

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('a setting another window changed', () => {
  it('is taken over rather than overwritten', async () => {
    loadedWith({ [SETTINGS_KEY]: JSON.stringify({ logView: 'pretty' }) });
    expect(useSettingsStore.getState().logView).toBe('pretty');
    served({ [SETTINGS_KEY]: JSON.stringify({ logView: 'raw' }) });

    await catchUp();

    expect(useSettingsStore.getState().logView).toBe('raw');
  });

  it('survives this window changing a different setting', async () => {
    loadedWith({ [SETTINGS_KEY]: JSON.stringify({ logView: 'pretty' }) });
    served({ [SETTINGS_KEY]: JSON.stringify({ logView: 'raw' }) });
    await catchUp();

    useSettingsStore.getState().setScreenReader(true);

    const written = JSON.parse(readStored(SETTINGS_KEY) ?? '{}') as Record<string, unknown>;
    expect(written.logView).toBe('raw');
    expect(written.screenReader).toBe(true);
  });
});

describe('a change this window has not saved yet', () => {
  it('is not undone by what the server still holds', async () => {
    startSaving();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('offline')));
    writeStored(THEME_KEY, 'borg');
    await flush();

    served({ [THEME_KEY]: 'nord' });
    await catchUp();

    expect(readStored(THEME_KEY)).toBe('borg');
  });
});

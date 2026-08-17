import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  SAVE_DELAY_MS,
  SETTINGS_PATH,
  forgetStored,
  hydrate,
  readStored,
  resetStored,
  save,
  startSaving,
  stopSaving,
  storedKeys,
  writeStored,
} from '../../src/lib/persist';

function served(values: Record<string, string>): void {
  window.__SPINOZA_SETTINGS__ = JSON.stringify(values);
}

function stubFetch() {
  const fetchMock = vi.fn((url: string, init?: { body?: string }) => {
    void url;
    void init;
    return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ values: {} }) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function body(fetchMock: ReturnType<typeof stubFetch>): Record<string, string> {
  const sent = fetchMock.mock.calls[0][1]?.body ?? '{}';
  return (JSON.parse(sent) as { values: Record<string, string> }).values;
}

beforeEach(() => {
  resetStored();
  delete window.__SPINOZA_SETTINGS__;
  window.localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  stopSaving();
  resetStored();
});

describe('the settings the server sent with the page', () => {
  it('are what the app reads', () => {
    served({ 'spinoza.theme.v1': '"nord"' });

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
  });

  it('are ignored when they are not readable', () => {
    window.__SPINOZA_SETTINGS__ = '{not json';
    window.localStorage.setItem('spinoza.theme.v1', '"matrix"');

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBe('"matrix"');
  });

  it('are ignored when they are not an object', () => {
    window.__SPINOZA_SETTINGS__ = '7';
    window.localStorage.setItem('spinoza.theme.v1', '"matrix"');

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBe('"matrix"');
  });

  it('skip anything that is not a plain string', () => {
    window.__SPINOZA_SETTINGS__ = JSON.stringify({ 'spinoza.theme.v1': 7 });

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBeNull();
  });
});

describe('settings left over in the browser', () => {
  it('are taken over the first time the server has none', () => {
    const fetchMock = stubFetch();
    startSaving();
    window.localStorage.setItem('spinoza.theme.v1', '"nord"');
    window.localStorage.setItem('spinoza.layout.v1', '{"sidebar":320}');
    served({});

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(body(fetchMock)).toEqual({
      'spinoza.theme.v1': '"nord"',
      'spinoza.layout.v1': '{"sidebar":320}',
    });
  });

  it('are left alone once the server has its own', () => {
    stubFetch();
    startSaving();
    window.localStorage.setItem('spinoza.theme.v1', '"matrix"');
    served({ 'spinoza.theme.v1': '"nord"' });

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
  });

  it('are read straight through when no page settings arrived at all', () => {
    window.localStorage.setItem('spinoza.theme.v1', '"nord"');

    hydrate();

    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
  });

  it('are skipped when the browser refuses to hand them over', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });

    hydrate();

    expect(storedKeys()).toEqual([]);
  });

  it('start nothing when there were none to take over', () => {
    const fetchMock = stubFetch();
    startSaving();
    served({});

    hydrate();

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('keeping settings', () => {
  it('sends them to the server once the typing settles', () => {
    vi.useFakeTimers();
    const fetchMock = stubFetch();
    startSaving();

    writeStored('spinoza.theme.v1', '"nord"');
    writeStored('spinoza.theme.v1', '"matrix"');
    expect(fetchMock).not.toHaveBeenCalled();

    vi.advanceTimersByTime(SAVE_DELAY_MS);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock.mock.calls[0][0]).toContain(SETTINGS_PATH);
    expect(body(fetchMock)).toEqual({ 'spinoza.theme.v1': '"matrix"' });
  });

  it('sends nothing while the app is not running in a page', () => {
    vi.useFakeTimers();
    const fetchMock = stubFetch();

    writeStored('spinoza.theme.v1', '"nord"');
    vi.advanceTimersByTime(SAVE_DELAY_MS);

    expect(fetchMock).not.toHaveBeenCalled();
    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
  });

  it('carries on when the server refuses the write', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    );
    startSaving();
    writeStored('spinoza.theme.v1', '"nord"');

    save();
    await Promise.resolve();
    await Promise.resolve();

    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
  });

  it('sends nothing on a direct save while the app is not running in a page', () => {
    const fetchMock = stubFetch();
    writeStored('spinoza.theme.v1', '"nord"');

    save();

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('starts a fresh set when the page holds none', () => {
    writeStored('spinoza.theme.v1', '"nord"');
    delete window.__spinozaSettings__;

    expect(readStored('spinoza.theme.v1')).toBeNull();
    expect(storedKeys()).toEqual([]);
  });

  it('forgets a setting on request', () => {
    vi.useFakeTimers();
    const fetchMock = stubFetch();
    startSaving();
    writeStored('spinoza.theme.v1', '"nord"');

    forgetStored('spinoza.theme.v1');
    vi.advanceTimersByTime(SAVE_DELAY_MS);

    expect(readStored('spinoza.theme.v1')).toBeNull();
    expect(body(fetchMock)).toEqual({});
  });

  it('lists what it holds', () => {
    writeStored('spinoza.theme.v1', '"nord"');
    writeStored('spinoza.sidebar.v1', '{}');

    expect(storedKeys().sort()).toEqual(['spinoza.sidebar.v1', 'spinoza.theme.v1']);
  });
});

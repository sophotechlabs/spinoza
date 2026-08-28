import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  SAVE_DELAY_MS,
  SETTINGS_PATH,
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

    await save();

    expect(readStored('spinoza.theme.v1')).toBe('"nord"');
  });

  it('sends nothing on a direct save while the app is not running in a page', () => {
    const fetchMock = stubFetch();
    writeStored('spinoza.theme.v1', '"nord"');

    void save();

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('starts a fresh set when the page holds none', () => {
    writeStored('spinoza.theme.v1', '"nord"');
    delete window.__spinozaSettings__;

    expect(readStored('spinoza.theme.v1')).toBeNull();
    expect(storedKeys()).toEqual([]);
  });

  it('lists what it holds', () => {
    writeStored('spinoza.theme.v1', '"nord"');
    writeStored('spinoza.sidebar.v1', '{}');

    expect(storedKeys().sort()).toEqual(['spinoza.sidebar.v1', 'spinoza.theme.v1']);
  });
});

describe('sending only what this window changed', () => {
  it('leaves out what it never touched', async () => {
    served({ 'spinoza.theme.v1': '"nord"', 'spinoza.layout.v1': '{"left":200}' });
    hydrate();
    const fetchMock = stubFetch();
    startSaving();

    writeStored('spinoza.theme.v1', '"borg"');
    await save();

    expect(body(fetchMock)).toEqual({ 'spinoza.theme.v1': '"borg"' });
  });

  it('sends nothing at all when nothing was written', async () => {
    served({ 'spinoza.theme.v1': '"nord"' });
    hydrate();
    const fetchMock = stubFetch();
    startSaving();

    await save();

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('stops sending a key once the server took it', async () => {
    const fetchMock = stubFetch();
    startSaving();
    writeStored('spinoza.theme.v1', '"borg"');
    await save();

    await save();

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  // A refused save has to come round again, or the change is lost quietly.
  it('keeps a key the server refused', async () => {
    const fetchMock = vi.fn().mockResolvedValue({ ok: false, status: 500 });
    vi.stubGlobal('fetch', fetchMock);
    startSaving();
    writeStored('spinoza.theme.v1', '"borg"');
    await save();

    await save();

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('keeps a key the server never answered about', async () => {
    const fetchMock = vi.fn().mockRejectedValue(new Error('offline'));
    vi.stubGlobal('fetch', fetchMock);
    startSaving();
    writeStored('spinoza.theme.v1', '"borg"');
    await save();

    await save();

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  // Written again while the first request was still out: the newer value has to
  // follow it rather than being forgotten with the older one.
  it('keeps a key written again while the save was in flight', async () => {
    const fetchMock = stubFetch();
    startSaving();
    writeStored('spinoza.theme.v1', '"borg"');
    const inFlight = save();

    writeStored('spinoza.theme.v1', '"nord"');
    await inFlight;
    await save();

    expect(fetchMock).toHaveBeenCalledTimes(2);
    const second = fetchMock.mock.calls[1][1] as { body: string };
    expect((JSON.parse(second.body) as { values: Record<string, string> }).values).toEqual({
      'spinoza.theme.v1': '"nord"',
    });
  });
});

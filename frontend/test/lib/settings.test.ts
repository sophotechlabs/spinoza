import { afterEach, describe, expect, it, vi } from 'vitest';
import { readStored, resetStored, startSaving, stopSaving } from '../../src/lib/persist';
import {
  NODE_SHELL_KEY,
  SETTINGS_KEY,
  parseSettings,
  readNodeShell,
  readSettings,
  writeNodeShell,
  writeSettings,
} from '../../src/lib/settings';
import type { Settings } from '../../src/lib/settings';

const base: Settings = {
  logView: 'pretty',
  screenReader: false,
  namespaceStart: 'all',
  namespaceStarts: {},
  checksInterval: 60,
};

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  stopSaving();
  resetStored();
  window.localStorage.clear();
});

describe('parseSettings', () => {
  it('accepts a stored log view', () => {
    expect(parseSettings(JSON.stringify({ logView: 'raw' })).logView).toBe('raw');
    expect(parseSettings(JSON.stringify({ logView: 'pretty' })).logView).toBe('pretty');
  });

  it('falls back to pretty for anything it cannot use', () => {
    expect(parseSettings(null).logView).toBe('pretty');
    expect(parseSettings('not json').logView).toBe('pretty');
    expect(parseSettings('null').logView).toBe('pretty');
    expect(parseSettings(JSON.stringify({ logView: 'sideways' })).logView).toBe('pretty');
    expect(parseSettings(JSON.stringify({ logView: 7 })).logView).toBe('pretty');
  });
});

describe('settings that outlive the tab', () => {
  it('round-trip through storage', () => {
    writeSettings({ ...base, logView: 'raw' });

    expect(readStored(SETTINGS_KEY)).toContain('"logView":"raw"');
    expect(readSettings().logView).toBe('raw');
  });

  it('fall back when storage refuses to be read', () => {
    vi.spyOn(window.localStorage, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });

    expect(readSettings().logView).toBe('pretty');
  });

  it('give up quietly when storage refuses to be written', () => {
    vi.spyOn(window.localStorage, 'setItem').mockImplementation(() => {
      throw new Error('denied');
    });

    expect(() => {
      writeSettings({ ...base, logView: 'raw' });
    }).not.toThrow();
  });
});

describe('the screen reader setting', () => {
  it('is off unless something stored says otherwise', () => {
    expect(parseSettings(null).screenReader).toBe(false);
    expect(parseSettings('{"logView":"raw"}').screenReader).toBe(false);
  });

  it('is read back when it was stored', () => {
    expect(parseSettings('{"screenReader":true}').screenReader).toBe(true);
  });

  it('ignores a value that is not a boolean', () => {
    expect(parseSettings('{"screenReader":"yes"}').screenReader).toBe(false);
  });
});

describe('the namespace to open on', () => {
  it('is every namespace unless something stored says otherwise', () => {
    expect(parseSettings(null).namespaceStart).toBe('all');
    expect(parseSettings('{"namespaceStart":"sideways"}').namespaceStart).toBe('all');
  });

  it('is read back when it was stored', () => {
    expect(parseSettings('{"namespaceStart":"default"}').namespaceStart).toBe('default');
  });

  it('remembers what each cluster was told to open on', () => {
    expect(parseSettings(null).namespaceStarts).toEqual({});
    expect(parseSettings('{"namespaceStarts":{"p-mk1":"default"}}').namespaceStarts).toEqual({
      'p-mk1': 'default',
    });
  });

  it('keeps only the answers it recognises', () => {
    const stored = '{"namespaceStarts":{"p-mk1":"default","gke":"sometimes","p-mk2":"all"}}';

    expect(parseSettings(stored).namespaceStarts).toEqual({ 'p-mk1': 'default', 'p-mk2': 'all' });
  });

  it('has no answers to read out of a broken map', () => {
    expect(parseSettings('{"namespaceStarts":"nope"}').namespaceStarts).toEqual({});
    expect(parseSettings('{"namespaceStarts":null}').namespaceStarts).toEqual({});
  });
});

describe('the node shell setting', () => {
  it('is off until something turns it on', () => {
    expect(readNodeShell()).toBe(false);
  });

  it('is read back on, and written where the server looks for it', async () => {
    startSaving();

    await writeNodeShell(true);

    expect(readStored(NODE_SHELL_KEY)).toBe('on');
    expect(readNodeShell()).toBe(true);
  });

  it('says off in as many words when it is turned back off', async () => {
    startSaving();
    await writeNodeShell(true);

    await writeNodeShell(false);

    expect(readStored(NODE_SHELL_KEY)).toBe('off');
    expect(readNodeShell()).toBe(false);
  });

  it('reaches the server before it resolves', async () => {
    const fetchMock = vi.fn((url: string, init?: { body?: string }) => {
      void url;
      void init;
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ values: {} }),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    startSaving();

    await writeNodeShell(true);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const sent = fetchMock.mock.calls[0][1]?.body ?? '{}';
    expect(JSON.parse(sent)).toMatchObject({ values: { [NODE_SHELL_KEY]: 'on' } });
  });
});

describe('the checks interval', () => {
  it('defaults to a minute when nothing is stored', () => {
    expect(parseSettings(null).checksInterval).toBe(60);
  });

  it('keeps an interval it offers', () => {
    expect(parseSettings(JSON.stringify({ ...base, checksInterval: 15 })).checksInterval).toBe(15);
  });

  it('refuses one it does not offer', () => {
    expect(parseSettings(JSON.stringify({ ...base, checksInterval: 7 })).checksInterval).toBe(60);
    expect(parseSettings(JSON.stringify({ ...base, checksInterval: 'soon' })).checksInterval).toBe(
      60,
    );
  });
});

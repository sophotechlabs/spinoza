import { afterEach, describe, expect, it, vi } from 'vitest';
import { SETTINGS_KEY, parseSettings, readSettings, writeSettings } from '../../src/lib/settings';

afterEach(() => {
  vi.restoreAllMocks();
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
    writeSettings({ logView: 'raw' });

    expect(window.localStorage.getItem(SETTINGS_KEY)).toContain('"logView":"raw"');
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
      writeSettings({ logView: 'raw' });
    }).not.toThrow();
  });
});

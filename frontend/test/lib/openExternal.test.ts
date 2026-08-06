import { afterEach, describe, expect, it, vi } from 'vitest';
import { forwardURL, openExternal } from '../../src/lib/openExternal';

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(window, 'runtime');
});

describe('forwardURL', () => {
  it('addresses the loopback port the forward listens on', () => {
    expect(forwardURL(45123)).toBe('http://127.0.0.1:45123');
  });
});

describe('openExternal', () => {
  it('hands the url to the real browser when running in the desktop app', () => {
    const browserOpenURL = vi.fn();
    const opened = vi.fn();
    vi.stubGlobal('open', opened);
    Reflect.set(window, 'runtime', { BrowserOpenURL: browserOpenURL });

    openExternal('http://127.0.0.1:45123');

    expect(browserOpenURL).toHaveBeenCalledWith('http://127.0.0.1:45123');
    expect(opened).not.toHaveBeenCalled();
  });

  it('opens a tab when running in a browser', () => {
    const opened = vi.fn();
    vi.stubGlobal('open', opened);

    openExternal('http://127.0.0.1:45123');

    expect(opened).toHaveBeenCalledWith('http://127.0.0.1:45123', '_blank', 'noreferrer');
  });

  it('falls back when the desktop runtime is there without the opener', () => {
    const opened = vi.fn();
    vi.stubGlobal('open', opened);
    Reflect.set(window, 'runtime', {});

    openExternal('http://127.0.0.1:45123');

    expect(opened).toHaveBeenCalledWith('http://127.0.0.1:45123', '_blank', 'noreferrer');
  });
});

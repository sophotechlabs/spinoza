import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  BROWSER,
  DESKTOP,
  fetchView,
  inDesktopWindow,
  moveToBrowser,
  moveToDesktop,
  viewKind,
} from '../../src/lib/view';

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn((url: string, init?: { method?: string }) => {
    void url;
    void init;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

afterEach(() => {
  vi.unstubAllGlobals();
  delete window.__SPINOZA_VIEW__;
});

describe('which view this is', () => {
  it('is a browser tab unless the page says otherwise', () => {
    expect(viewKind()).toBe(BROWSER);
    expect(inDesktopWindow()).toBe(false);
  });

  it('is the desktop window when the page says so', () => {
    window.__SPINOZA_VIEW__ = DESKTOP;

    expect(viewKind()).toBe(DESKTOP);
    expect(inDesktopWindow()).toBe(true);
  });

  it('is a browser tab for anything it does not recognise', () => {
    window.__SPINOZA_VIEW__ = 'kiosk';

    expect(viewKind()).toBe(BROWSER);
  });
});

describe('fetchView', () => {
  it('reports whether this build has a window', async () => {
    stub({ window: true, hidden: true });

    await expect(fetchView()).resolves.toEqual({ window: true, hidden: true });
  });

  it('fills in what the backend left out', async () => {
    stub({});

    await expect(fetchView()).resolves.toEqual({ window: false, hidden: false });
  });

  it('reports a lookup that failed', async () => {
    stub({ message: 'no view here' }, false, 500);

    await expect(fetchView()).rejects.toThrow('no view here');
  });
});

describe('moving between views', () => {
  it('asks the server to open the browser', async () => {
    const fetchMock = stub({ switched: true });

    await expect(moveToBrowser()).resolves.toEqual({ switched: true, reason: undefined });
    expect(fetchMock.mock.calls[0][0]).toContain('/api/view/browser');
    expect(fetchMock.mock.calls[0][1]).toMatchObject({ method: 'POST' });
  });

  it('passes on the reason the window stayed', async () => {
    stub({ switched: false, reason: 'the browser did not open spinoza' });

    await expect(moveToBrowser()).resolves.toEqual({
      switched: false,
      reason: 'the browser did not open spinoza',
    });
  });

  it('asks the server for the window back', async () => {
    const fetchMock = stub({ switched: true });

    await expect(moveToDesktop()).resolves.toEqual({ switched: true, reason: undefined });
    expect(fetchMock.mock.calls[0][0]).toContain('/api/view/desktop');
  });

  it('reports a switch the backend refused', async () => {
    stub({ message: 'this build has no desktop window' }, false, 501);

    await expect(moveToDesktop()).rejects.toThrow('no desktop window');
  });
});

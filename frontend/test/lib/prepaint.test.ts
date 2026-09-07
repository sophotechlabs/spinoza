import { afterEach, describe, expect, it } from 'vitest';
import html from '../../index.html?raw';
import { DEFAULT_THEME_FILE } from '../../src/lib/defaultTheme';
import { PAINTED_KEY, THEME_KEY } from '../../src/lib/theme';

function inlineScript(): string {
  const match = /<script>([\s\S]*?)<\/script>/.exec(html);
  if (match === null) {
    throw new Error('index.html has no inline pre-paint script');
  }
  return match[1];
}

function served(values: Record<string, string>): void {
  window.__SPINOZA_SETTINGS__ = JSON.stringify(values);
}

function prepaint(): void {
  const code = inlineScript().replace('__DEFAULT_THEME__', JSON.stringify(DEFAULT_THEME_FILE));
  (0, eval)(code);
}

function surface(): string {
  return document.documentElement.style.getPropertyValue('--surface');
}

afterEach(() => {
  document.documentElement.removeAttribute('style');
  delete document.documentElement.dataset.theme;
});

describe('the paint before the app loads', () => {
  it('is the default theme when nothing was ever chosen', () => {
    prepaint();

    expect(document.documentElement.dataset.theme).toBe(DEFAULT_THEME_FILE.base);
    expect(surface()).toBe(DEFAULT_THEME_FILE.tokens.surface);
  });

  it('is the plain base for a theme whose colours live in the css', () => {
    served({ [THEME_KEY]: 'light' });

    prepaint();

    expect(document.documentElement.dataset.theme).toBe('light');
    expect(surface()).toBe('');
  });

  it('replays the last paint for a theme that was chosen', () => {
    served({
      [THEME_KEY]: 'nord',
      [PAINTED_KEY]: JSON.stringify({ base: 'dark', tokens: { surface: '#2e3440' } }),
    });

    prepaint();

    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(surface()).toBe('#2e3440');
  });

  it('leaves a chosen theme that was never painted to the app', () => {
    served({ [THEME_KEY]: 'nord' });

    prepaint();

    expect(document.documentElement.dataset.theme).toBe('dark');
    expect(surface()).toBe('');
  });

  it('replays the last paint over the default when both are known', () => {
    served({
      [THEME_KEY]: DEFAULT_THEME_FILE.id,
      [PAINTED_KEY]: JSON.stringify({ base: 'dark', tokens: { surface: '#2e3440' } }),
    });

    prepaint();

    expect(surface()).toBe('#2e3440');
  });
});

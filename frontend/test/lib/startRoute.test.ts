import { afterEach, describe, expect, it } from 'vitest';
import { startRoute } from '../../src/lib/view';

function ask(start: { view?: string; context?: string } | undefined, hash = ''): string {
  window.location.hash = hash;
  window.__SPINOZA_START__ = start;
  return startRoute();
}

afterEach(() => {
  window.location.hash = '';
  delete window.__SPINOZA_START__;
});

describe('startRoute', () => {
  it('is nothing when the run asked for nothing', () => {
    expect(ask(undefined)).toBe('');
  });

  it('names the view and the context the run asked for', () => {
    const got = ask({ view: 'traffic', context: 'p-mk1' });

    expect(got).toContain('view=traffic');
    expect(got).toContain('context=p-mk1');
    expect(got.startsWith('#')).toBe(true);
  });

  it('leaves a hash the window already carries alone', () => {
    expect(ask({ view: 'traffic' }, '#context=p-mk2&view=checks')).toBe('');
  });

  it('drops a view this build does not know', () => {
    expect(ask({ view: 'nowhere' })).toBe('');
  });

  it('keeps the context even when the view is unknown', () => {
    expect(ask({ view: 'nowhere', context: 'p-mk1' })).toBe('#context=p-mk1');
  });

  it('is nothing when the run asked for empty strings', () => {
    expect(ask({ view: '', context: '' })).toBe('');
  });

  it('takes a context on its own', () => {
    expect(ask({ context: 'p-mk1' })).toBe('#context=p-mk1');
  });
});

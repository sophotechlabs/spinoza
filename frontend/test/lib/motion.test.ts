import { afterEach, describe, expect, it, vi } from 'vitest';
import { REDUCED_MOTION, prefersReducedMotion } from '../../src/lib/motion';
import { installMatchMedia } from '../helpers';

afterEach(() => {
  installMatchMedia();
});

describe('prefersReducedMotion', () => {
  it('follows the media query', () => {
    vi.stubGlobal('matchMedia', (query: string) => ({ matches: query === REDUCED_MOTION }));

    expect(prefersReducedMotion()).toBe(true);
  });

  it('is false when the user asked for no such thing', () => {
    vi.stubGlobal('matchMedia', () => ({ matches: false }));

    expect(prefersReducedMotion()).toBe(false);
  });

  it('is false where matchMedia is missing rather than throwing', () => {
    vi.stubGlobal('matchMedia', () => {
      throw new Error('not implemented');
    });

    expect(prefersReducedMotion()).toBe(false);
  });
});

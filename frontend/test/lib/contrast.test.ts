import { describe, expect, it } from 'vitest';
import { contrastRatio, contrastWarnings, toHex, toRgb } from '../../src/lib/contrast';

describe('reading a colour', () => {
  it('agrees with the browser on the values the CI gate also pins', () => {
    expect(toRgb('oklch(52.7% 0.154 150.069)')).toEqual([0, 130, 54]);
    expect(toRgb('oklch(100% 0 none)')).toEqual([255, 255, 255]);
    expect(toRgb('oklch(14.5% 0 none)')).toEqual([10, 10, 10]);
  });

  it('reads hex in every length a person writes', () => {
    expect(toRgb('#fff')).toEqual([255, 255, 255]);
    expect(toRgb('#fdf6e3')).toEqual([253, 246, 227]);
    expect(toRgb('#fdf6e380')).toEqual([253, 246, 227]);
    expect(toRgb('#0a0a0a')).toEqual([10, 10, 10]);
  });

  it('reads rgb notation', () => {
    expect(toRgb('rgb(10, 20, 30)')).toEqual([10, 20, 30]);
    expect(toRgb('rgba(10 20 30 / 0.5)')).toEqual([10, 20, 30]);
  });

  it('returns nothing for a notation it cannot read', () => {
    expect(toRgb('rebeccapurple')).toBeNull();
    expect(toRgb('rgb(1)')).toBeNull();
    expect(toRgb('#fffff')).toBeNull();
  });
});

describe('contrastRatio', () => {
  it('puts black on white at the top of the scale', () => {
    expect(contrastRatio([0, 0, 0], [255, 255, 255])).toBeCloseTo(21, 1);
  });

  it('gives a colour against itself the bottom of the scale', () => {
    expect(contrastRatio([10, 10, 10], [10, 10, 10])).toBeCloseTo(1, 5);
  });
});

describe('warning about a theme that is hard to read', () => {
  const nothing = () => '';

  function lightBase(name: string): string {
    if (name.startsWith('surface')) {
      return '#ffffff';
    }
    if (name.endsWith('-tint')) {
      return '#ffffff';
    }
    return '#111111';
  }

  it('names each token that falls below AA and the background it lands on', () => {
    const warnings = contrastWarnings(
      {
        surface: '#ffffff',
        'surface-raised': '#ffffff',
        'surface-active': '#ffffff',
        fg: '#eeeeee',
      },
      nothing,
    );

    expect(warnings).toHaveLength(3);
    expect(warnings[0]).toMatch(/^fg is \d+\.\d+:1 on surface, below AA$/);
    expect(warnings[2]).toMatch(/on surface-active, below AA$/);
  });

  it('measures an override against the background the base theme supplies', () => {
    const warnings = contrastWarnings({ fg: '#eeeeee' }, lightBase);

    expect(warnings.some((line) => line.startsWith('fg is'))).toBe(true);
  });

  it('measures a background override against the text the base theme supplies', () => {
    function dark(name: string): string {
      if (name === 'fg-subtle') {
        return '#777777';
      }
      return '#000000';
    }
    const warnings = contrastWarnings({ 'surface-active': '#888888' }, dark);

    expect(warnings.some((line) => line.includes('on surface-active'))).toBe(true);
  });

  it('keeps quiet when every colour is legible', () => {
    expect(contrastWarnings({ fg: '#111111' }, lightBase)).toEqual([]);
  });

  it('says nothing when nothing can be read', () => {
    expect(contrastWarnings({ fg: '#eeeeee' }, nothing)).toEqual([]);
  });

  it('skips a token it cannot read rather than guessing', () => {
    expect(contrastWarnings({ fg: 'beige' }, lightBase)).toEqual([]);
  });

  it('ignores tokens that are not text', () => {
    expect(contrastWarnings({ 'ok-tint': '#fefefe' }, lightBase)).toEqual([]);
  });
});

describe('toHex', () => {
  it('passes a hex colour straight through, normalised', () => {
    expect(toHex('#f00')).toBe('#ff0000');
    expect(toHex('#00FF00')).toBe('#00ff00');
  });

  it('converts oklch to hex so a canvas renderer can use it', () => {
    expect(toHex('oklch(79.2% 0.209 151.711)')).toMatch(/^#[0-9a-f]{6}$/);
  });

  it('gives nothing back for something it cannot read', () => {
    expect(toHex('rebeccapurple')).toBeNull();
    expect(toHex('')).toBeNull();
  });
});

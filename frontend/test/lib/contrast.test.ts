import { describe, expect, it } from 'vitest';
import { contrastRatio, contrastWarnings, toRgb } from '../../src/lib/contrast';

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
  it('names each token that falls below AA against the surface it was given', () => {
    const warnings = contrastWarnings({ surface: '#ffffff', fg: '#eeeeee', ok: '#008236' });

    expect(warnings).toHaveLength(1);
    expect(warnings[0]).toMatch(/^fg is \d+\.\d+:1 against your surface, below AA$/);
  });

  it('keeps quiet when every colour is legible', () => {
    expect(contrastWarnings({ surface: '#ffffff', fg: '#111111' })).toEqual([]);
  });

  it('says nothing when the theme sets no surface to measure against', () => {
    expect(contrastWarnings({ fg: '#eeeeee' })).toEqual([]);
  });

  it('says nothing when the surface itself cannot be read', () => {
    expect(contrastWarnings({ surface: 'beige', fg: '#eeeeee' })).toEqual([]);
  });

  it('skips a token it cannot read rather than guessing', () => {
    expect(contrastWarnings({ surface: '#ffffff', fg: 'beige' })).toEqual([]);
  });

  it('ignores tokens that are not text', () => {
    expect(contrastWarnings({ surface: '#ffffff', 'ok-tint': '#fefefe' })).toEqual([]);
  });
});

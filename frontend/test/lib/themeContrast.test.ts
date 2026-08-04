import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

const CSS = readFileSync('src/index.css', 'utf8');

const CONTENT_TOKENS = [
  'fg-strong',
  'fg',
  'fg-soft',
  'fg-muted',
  'fg-subtle',
  'ok',
  'ok-contrast',
  'error',
  'error-strong',
  'error-contrast',
  'warn',
  'warn-strong',
  'warn-muted',
  'info-contrast',
  'ansi-black',
  'ansi-red',
  'ansi-green',
  'ansi-yellow',
  'ansi-blue',
  'ansi-magenta',
  'ansi-cyan',
  'ansi-white',
  'ansi-bright-black',
  'ansi-bright-red',
  'ansi-bright-green',
  'ansi-bright-yellow',
  'ansi-bright-blue',
  'ansi-bright-magenta',
  'ansi-bright-cyan',
  'ansi-bright-white',
];

const DARK_EXEMPT = new Set(['ansi-black', 'ansi-bright-black', 'fg-subtle']);

function block(selector: string): string {
  const at = CSS.indexOf(selector);
  if (at < 0) {
    throw new Error(`missing ${selector}`);
  }
  const open = CSS.indexOf('{', at);
  const close = CSS.indexOf('\n}', open);
  return CSS.slice(open, close);
}

function tokens(selector: string): Map<string, string> {
  const found = new Map<string, string>();
  const pattern = /--([a-z0-9-]+):\s*([^;]+);/g;
  let match = pattern.exec(block(selector));
  while (match !== null) {
    found.set(match[1], match[2].trim());
    match = pattern.exec(block(selector));
  }
  return found;
}

function oklchToRgb(value: string): [number, number, number] {
  const parts = /oklch\(\s*([\d.]+)%\s+([\d.]+)\s+([\d.]+|none)\s*\)/.exec(value);
  if (parts === null) {
    throw new Error(`not oklch: ${value}`);
  }
  const L = Number(parts[1]) / 100;
  const C = Number(parts[2]);
  let h = 0;
  if (parts[3] !== 'none') {
    h = (Number(parts[3]) * Math.PI) / 180;
  }
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);

  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;

  const lin = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];

  return lin.map((c) => {
    let v = c;
    if (v <= 0.0031308) {
      v = 12.92 * v;
    } else {
      v = 1.055 * Math.pow(v, 1 / 2.4) - 0.055;
    }
    return Math.min(255, Math.max(0, Math.round(v * 255)));
  }) as [number, number, number];
}

function luminance([r, g, b]: [number, number, number]): number {
  const channel = (raw: number) => {
    const c = raw / 255;
    if (c <= 0.03928) {
      return c / 12.92;
    }
    return Math.pow((c + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function contrast(a: [number, number, number], b: [number, number, number]): number {
  const first = luminance(a);
  const second = luminance(b);
  return (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05);
}

function ratiosFor(selector: string): Map<string, number> {
  const values = tokens(selector);
  const surfaceValue = values.get('surface');
  if (surfaceValue === undefined) {
    throw new Error(`no surface in ${selector}`);
  }
  const surface = oklchToRgb(surfaceValue);
  const out = new Map<string, number>();
  for (const token of CONTENT_TOKENS) {
    const raw = values.get(token);
    if (raw === undefined) {
      throw new Error(`no --${token} in ${selector}`);
    }
    out.set(token, contrast(oklchToRgb(raw), surface));
  }
  return out;
}

describe('the colour maths this file relies on', () => {
  it('agrees with what the browser computes for a known token', () => {
    expect(oklchToRgb('oklch(52.7% 0.154 150.069)')).toEqual([0, 130, 54]);
    expect(oklchToRgb('oklch(100% 0 none)')).toEqual([255, 255, 255]);
    expect(oklchToRgb('oklch(14.5% 0 none)')).toEqual([10, 10, 10]);
  });
});

describe('text a person actually has to read', () => {
  it('clears WCAG AA against the light surface', () => {
    const ratios = ratiosFor(":root[data-theme='light']");
    const failing = [...ratios].filter(([, ratio]) => ratio < 4.5).map(([token]) => token);

    expect(failing).toEqual([]);
  });

  it('clears WCAG AA against the dark surface, bar the greys inherited from before theming', () => {
    const ratios = ratiosFor(':root {');
    const failing = [...ratios]
      .filter(([token, ratio]) => ratio < 4.5 && !DARK_EXEMPT.has(token))
      .map(([token]) => token);

    expect(failing).toEqual([]);
  });
});

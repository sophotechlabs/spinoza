import { CONTENT_TOKENS, backgroundsFor } from './theme';

const AA = 4.5;

const HEX = /^#([0-9a-f]{3,8})$/i;
const RGB = /^rgba?\(([^()]+)\)$/i;
const OKLCH = /^oklch\(\s*([\d.]+)%?\s+([\d.]+)\s+([\d.]+|none)\s*(?:\/.*)?\)$/i;

type Rgb = [number, number, number];

function fromHex(digits: string): Rgb | null {
  let text = digits;
  if (text.length === 3 || text.length === 4) {
    text = text
      .slice(0, 3)
      .split('')
      .map((char) => char + char)
      .join('');
  }
  if (text.length === 8) {
    text = text.slice(0, 6);
  }
  if (text.length !== 6) {
    return null;
  }
  return [
    parseInt(text.slice(0, 2), 16),
    parseInt(text.slice(2, 4), 16),
    parseInt(text.slice(4, 6), 16),
  ];
}

function clamp(value: number): number {
  return Math.min(255, Math.max(0, Math.round(value * 255)));
}

function fromOklch(lightness: number, chroma: number, hueDeg: number): Rgb {
  const hue = (hueDeg * Math.PI) / 180;
  const a = chroma * Math.cos(hue);
  const b = chroma * Math.sin(hue);
  const l = (lightness + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (lightness - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (lightness - 0.0894841775 * a - 1.291485548 * b) ** 3;
  const linear = [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
  const [r, g, blue] = linear.map((channel) => {
    if (channel <= 0.0031308) {
      return clamp(12.92 * channel);
    }
    return clamp(1.055 * Math.pow(channel, 1 / 2.4) - 0.055);
  });
  return [r, g, blue];
}

export function toRgb(value: string): Rgb | null {
  const text = value.trim();

  const hex = HEX.exec(text);
  if (hex !== null) {
    return fromHex(hex[1]);
  }

  const rgb = RGB.exec(text);
  if (rgb !== null) {
    const parts = rgb[1].split(/[\s,/]+/).filter((part) => part !== '');
    if (parts.length < 3) {
      return null;
    }
    return [Number(parts[0]), Number(parts[1]), Number(parts[2])];
  }

  const oklch = OKLCH.exec(text);
  if (oklch !== null) {
    let lightness = Number(oklch[1]);
    if (text.includes('%')) {
      lightness = lightness / 100;
    }
    let hue = 0;
    if (oklch[3] !== 'none') {
      hue = Number(oklch[3]);
    }
    return fromOklch(lightness, Number(oklch[2]), hue);
  }

  return null;
}

function luminance([r, g, b]: Rgb): number {
  const channel = (raw: number) => {
    const c = raw / 255;
    if (c <= 0.03928) {
      return c / 12.92;
    }
    return Math.pow((c + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

export function toHex(value: string): string | null {
  const rgb = toRgb(value);
  if (rgb === null) {
    return null;
  }
  return `#${rgb.map((channel) => channel.toString(16).padStart(2, '0')).join('')}`;
}

export function contrastRatio(a: Rgb, b: Rgb): number {
  const first = luminance(a);
  const second = luminance(b);
  return (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05);
}

function fromDocument(name: string): string {
  return window.getComputedStyle(document.documentElement).getPropertyValue(`--${name}`);
}

export function contrastWarnings(
  tokens: Record<string, string | undefined>,
  base: (name: string) => string = fromDocument,
): string[] {
  function resolve(name: string): Rgb | null {
    const own = tokens[name];
    if (own !== undefined) {
      return toRgb(own);
    }
    return toRgb(base(name));
  }

  const warnings: string[] = [];
  for (const token of CONTENT_TOKENS) {
    const rgb = resolve(token);
    if (rgb === null) {
      continue;
    }
    for (const name of backgroundsFor(token)) {
      const behind = resolve(name);
      if (behind === null) {
        continue;
      }
      const ratio = contrastRatio(rgb, behind);
      if (ratio < AA) {
        warnings.push(`${token} is ${ratio.toFixed(2)}:1 on ${name}, below AA`);
      }
    }
  }
  return warnings;
}

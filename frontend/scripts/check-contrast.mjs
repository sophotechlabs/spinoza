import { readFileSync } from 'node:fs';

const AA = 4.5;

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

const THEMES = [
  { name: 'dark', selector: ':root {' },
  { name: 'light', selector: ":root[data-theme='light']" },
];

const css = readFileSync(new URL('../src/index.css', import.meta.url), 'utf8');

function block(selector) {
  const at = css.indexOf(selector);
  if (at < 0) {
    throw new Error(`missing ${selector} in src/index.css`);
  }
  const open = css.indexOf('{', at);
  const close = css.indexOf('\n}', open);
  return css.slice(open, close);
}

function tokens(selector) {
  const found = new Map();
  const pattern = /--([a-z0-9-]+):\s*([^;]+);/g;
  const body = block(selector);
  let match = pattern.exec(body);
  while (match !== null) {
    found.set(match[1], match[2].trim());
    match = pattern.exec(body);
  }
  return found;
}

function oklchToRgb(value) {
  const parts = /oklch\(\s*([\d.]+)%\s+([\d.]+)\s+([\d.]+|none)\s*\)/.exec(value);
  if (parts === null) {
    throw new Error(`not an oklch colour: ${value}`);
  }
  const L = Number(parts[1]) / 100;
  const C = Number(parts[2]);
  const h = parts[3] === 'none' ? 0 : (Number(parts[3]) * Math.PI) / 180;
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ].map((c) => {
    const v = c <= 0.0031308 ? 12.92 * c : 1.055 * Math.pow(c, 1 / 2.4) - 0.055;
    return Math.min(255, Math.max(0, Math.round(v * 255)));
  });
}

function luminance([r, g, b]) {
  const channel = (raw) => {
    const c = raw / 255;
    return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function contrast(a, b) {
  const first = luminance(a);
  const second = luminance(b);
  return (Math.max(first, second) + 0.05) / (Math.min(first, second) + 0.05);
}

function selfCheck() {
  const cases = [
    ['oklch(52.7% 0.154 150.069)', [0, 130, 54]],
    ['oklch(100% 0 none)', [255, 255, 255]],
    ['oklch(14.5% 0 none)', [10, 10, 10]],
  ];
  for (const [value, want] of cases) {
    const got = oklchToRgb(value);
    if (got.join(',') !== want.join(',')) {
      throw new Error(
        `colour maths drifted: ${value} gave ${got.join(',')}, expected ${want.join(',')}`,
      );
    }
  }
}

selfCheck();

function tokenNamesFromCss() {
  const names = [];
  const pattern = /^\s+--color-([a-z0-9-]+):/gm;
  let match = pattern.exec(css);
  while (match !== null) {
    names.push(match[1]);
    match = pattern.exec(css);
  }
  return names;
}

function tokenNamesFromSource() {
  const source = readFileSync(new URL('../src/lib/theme.ts', import.meta.url), 'utf8');
  const block = /export const TOKEN_NAMES = \[([\s\S]*?)\];/.exec(source);
  if (block === null) {
    throw new Error('TOKEN_NAMES not found in src/lib/theme.ts');
  }
  return [...block[1].matchAll(/'([a-z0-9-]+)'/g)].map((entry) => entry[1]);
}

const fromCss = tokenNamesFromCss();
const fromSource = tokenNamesFromSource();
const missing = fromCss.filter((name) => !fromSource.includes(name));
const extra = fromSource.filter((name) => !fromCss.includes(name));

if (missing.length > 0 || extra.length > 0) {
  console.error('TOKEN_NAMES has drifted from src/index.css:');
  for (const name of missing) {
    console.error(`  in the css but not in TOKEN_NAMES: ${name}`);
  }
  for (const name of extra) {
    console.error(`  in TOKEN_NAMES but not in the css: ${name}`);
  }
  process.exit(1);
}

const failures = [];
for (const theme of THEMES) {
  const values = tokens(theme.selector);
  const surface = oklchToRgb(values.get('surface'));
  for (const token of CONTENT_TOKENS) {
    const raw = values.get(token);
    if (raw === undefined) {
      throw new Error(`no --${token} in the ${theme.name} theme`);
    }
    const ratio = contrast(oklchToRgb(raw), surface);
    if (ratio < AA) {
      failures.push(`${theme.name}: --${token} is ${ratio.toFixed(2)}:1 against the surface`);
    }
  }
}

if (failures.length > 0) {
  console.error(`text below WCAG AA (${String(AA)}:1):`);
  for (const line of failures) {
    console.error(`  ${line}`);
  }
  process.exit(1);
}

console.log(
  `theme: ${String(fromCss.length)} tokens in step with the css, ` +
    `${String(CONTENT_TOKENS.length * THEMES.length)} token/theme pairs clear AA`,
);

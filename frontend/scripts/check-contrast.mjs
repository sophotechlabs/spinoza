import { readdirSync, readFileSync } from 'node:fs';

const AA = 4.5;

function namesFromSource(constName) {
  const source = readFileSync(new URL('../src/lib/theme.ts', import.meta.url), 'utf8');
  const block = new RegExp(`export const ${constName} = \\[([\\s\\S]*?)\\];`).exec(source);
  if (block === null) {
    throw new Error(`${constName} not found in src/lib/theme.ts`);
  }
  return [...block[1].matchAll(/'([a-z0-9-]+)'/g)].map((entry) => entry[1]);
}

const CONTENT_TOKENS = namesFromSource('CONTENT_TOKENS');
const SURFACE_TOKENS = namesFromSource('SURFACE_TOKENS');

function tintMap() {
  const source = readFileSync(new URL('../src/lib/theme.ts', import.meta.url), 'utf8');
  const block = /export const TINT_BACKGROUNDS[^=]*= \{([\s\S]*?)\};/.exec(source);
  if (block === null) {
    throw new Error('TINT_BACKGROUNDS not found in src/lib/theme.ts');
  }
  const map = new Map();
  for (const entry of block[1].matchAll(/'([a-z0-9-]+)':\s*'([a-z0-9-]+)'/g)) {
    map.set(entry[1], entry[2]);
  }
  return map;
}

const TINTS = tintMap();

function backgroundsFor(token) {
  const tint = TINTS.get(token);
  if (tint !== undefined) {
    return [tint];
  }
  if (token.startsWith('ansi-')) {
    return ['surface'];
  }
  return SURFACE_TOKENS;
}

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

function pairsFromSource(constName, arity) {
  const source = readFileSync(new URL('../src/lib/themeColors.ts', import.meta.url), 'utf8');
  const block = new RegExp(`const ${constName}[^=]*= \\[([\\s\\S]*?)\\];`).exec(source);
  if (block === null) {
    throw new Error(`${constName} not found in src/lib/themeColors.ts`);
  }
  const rows = [];
  for (const entry of block[1].matchAll(/\[([^\]]*)\]/g)) {
    const parts = [...entry[1].matchAll(/'([^']*)'/g)].map((one) => one[1]);
    if (parts.length !== arity) {
      throw new Error(
        `${constName} row has ${String(parts.length)} entries, expected ${String(arity)}`,
      );
    }
    rows.push(parts);
  }
  if (rows.length === 0) {
    throw new Error(`${constName} in src/lib/themeColors.ts is empty`);
  }
  return rows;
}

function editorPairs() {
  const pairs = [];
  for (const [key, token, behind] of pairsFromSource('EDITOR_TEXT', 3)) {
    pairs.push({ key, token, behind });
  }
  for (const [rule, token] of pairsFromSource('EDITOR_RULES', 2)) {
    let named = rule;
    if (named === '') {
      named = 'default';
    }
    pairs.push({ key: `rule ${named}`, token, behind: 'surface' });
  }
  return pairs;
}

const EDITOR_PAIRS = editorPairs();

function hexToRgb(value) {
  const short = /^#([0-9a-f])([0-9a-f])([0-9a-f])$/i.exec(value);
  if (short !== null) {
    return [1, 2, 3].map((at) => parseInt(short[at] + short[at], 16));
  }
  const long = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(value);
  if (long === null) {
    return null;
  }
  return [1, 2, 3].map((at) => parseInt(long[at], 16));
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

const fromCss = tokenNamesFromCss();
const fromSource = namesFromSource('TOKEN_NAMES');
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

function anyToRgb(value) {
  const hex = hexToRgb(value.trim());
  if (hex !== null) {
    return hex;
  }
  return oklchToRgb(value);
}

const BASE_TOKENS = {
  dark: tokens(':root {'),
  light: tokens(":root[data-theme='light']"),
};

function shippedThemes() {
  const dir = new URL('../themes/', import.meta.url);
  const found = [];
  for (const name of readdirSync(dir).sort()) {
    if (!name.endsWith('.json')) {
      continue;
    }
    found.push(JSON.parse(readFileSync(new URL(name, dir), 'utf8')));
  }
  if (found.length === 0) {
    throw new Error('no themes in frontend/themes');
  }
  return found;
}

function resolverFor(theme) {
  const base = BASE_TOKENS[theme.base];
  if (base === undefined) {
    throw new Error(`${theme.name} has base ${String(theme.base)}, which is not dark or light`);
  }
  return (token) => {
    const own = theme.tokens?.[token];
    if (own !== undefined) {
      return anyToRgb(own);
    }
    const inherited = base.get(token);
    if (inherited === undefined) {
      return null;
    }
    return anyToRgb(inherited);
  };
}

const failures = [];
let pairs = 0;
let editorChecked = 0;

for (const theme of shippedThemes()) {
  const resolve = resolverFor(theme);
  for (const token of CONTENT_TOKENS) {
    const ink = resolve(token);
    if (ink === null) {
      continue;
    }
    for (const name of backgroundsFor(token)) {
      const paper = resolve(name);
      if (paper === null) {
        continue;
      }
      pairs += 1;
      const ratio = contrast(ink, paper);
      if (ratio < AA) {
        failures.push(`${theme.name}: --${token} is ${ratio.toFixed(2)}:1 on --${name}`);
      }
    }
  }
  for (const { key, token, behind } of EDITOR_PAIRS) {
    const ink = resolve(token);
    if (ink === null) {
      continue;
    }
    const paper = resolve(behind);
    if (paper === null) {
      continue;
    }
    editorChecked += 1;
    const ratio = contrast(ink, paper);
    if (ratio < AA) {
      failures.push(
        `${theme.name}: editor ${key} takes --${token}, ${ratio.toFixed(2)}:1 on --${behind}`,
      );
    }
  }
}

for (const theme of THEMES) {
  const values = tokens(theme.selector);
  for (const token of CONTENT_TOKENS) {
    const raw = values.get(token);
    if (raw === undefined) {
      throw new Error(`no --${token} in the ${theme.name} theme`);
    }
    for (const name of backgroundsFor(token)) {
      const behind = values.get(name);
      if (behind === undefined) {
        throw new Error(`no --${name} in the ${theme.name} theme`);
      }
      pairs += 1;
      const ratio = contrast(oklchToRgb(raw), oklchToRgb(behind));
      if (ratio < AA) {
        failures.push(`${theme.name}: --${token} is ${ratio.toFixed(2)}:1 on --${name}`);
      }
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

const NON_TEXT = 3;

const APART = 24;

const SURFACES = ['surface', 'surface-raised', 'surface-active'];

function clusterHues(selector) {
  const found = [];
  for (const [name, value] of tokens(selector)) {
    if (name.startsWith('cluster-')) {
      found.push([name, value]);
    }
  }
  return found;
}

const clusterVars = {
  dark: clusterHues(':root {'),
  light: clusterHues(":root[data-theme='light']"),
};

const baseSurfaces = {
  dark: tokens(':root {'),
  light: tokens(":root[data-theme='light']"),
};

const everyTheme = [
  { id: 'dark', base: 'dark', tokens: {} },
  { id: 'light', base: 'light', tokens: {} },
  ...shippedThemes(),
];

const dim = [];
let checked = 0;
for (const theme of everyTheme) {
  const hues = clusterVars[theme.base];
  if (hues.length === 0) {
    throw new Error(`no --cluster-* colours for the ${theme.base} base`);
  }
  for (const surface of SURFACES) {
    const behind = theme.tokens?.[surface] ?? baseSurfaces[theme.base].get(surface);
    if (behind === undefined) {
      throw new Error(`no --${surface} for ${theme.id}`);
    }
    for (const [name, hue] of hues) {
      checked += 1;
      const ratio = contrast(anyToRgb(hue), anyToRgb(behind));
      if (ratio < NON_TEXT) {
        dim.push(`${theme.id}: --${name} is ${ratio.toFixed(2)}:1 on --${surface}`);
      }
    }
  }
}

const alike = [];
for (const [base, hues] of Object.entries(clusterVars)) {
  for (let a = 0; a < hues.length; a += 1) {
    for (let b = a + 1; b < hues.length; b += 1) {
      const one = anyToRgb(hues[a][1]);
      const two = anyToRgb(hues[b][1]);
      const gap = Math.max(...one.map((channel, at) => Math.abs(channel - two[at])));
      if (gap < APART) {
        alike.push(`${base}: --${hues[a][0]} and --${hues[b][0]} are ${String(gap)} apart`);
      }
    }
  }
}

if (dim.length > 0 || alike.length > 0) {
  console.error(`cluster colours below ${String(NON_TEXT)}:1, or too close to each other:`);
  for (const line of [...dim, ...alike]) {
    console.error(`  ${line}`);
  }
  process.exit(1);
}

console.log(
  `theme: ${String(fromCss.length)} tokens in step with the css, ` +
    `${String(pairs)} token/background pairs clear AA, ` +
    `${String(editorChecked)} of them in the editor, ` +
    `${String(checked)} cluster colour/surface pairs clear ${String(NON_TEXT)}:1 ` +
    `across ${String(everyTheme.length)} themes`,
);

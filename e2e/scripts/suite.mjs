import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const e2e = resolve(here, '..');

export const FULL = 'full';
export const OWNED = 'owned';
export const UNIT_ONLY = 'unit-only';
export const UNMAPPED = 'unmapped';
export const UNRELATED = 'unrelated';

export function loadSuite() {
  return JSON.parse(readFileSync(resolve(e2e, 'suite.json'), 'utf8'));
}

export function groupByID(suite, id) {
  const group = suite.groups.find((candidate) => candidate.id === id);
  if (group === undefined) {
    throw new Error(`unknown E2E group ${id}`);
  }
  return group;
}

function patternExpression(pattern) {
  let expression = '^';
  for (let index = 0; index < pattern.length; index += 1) {
    const char = pattern[index];
    if (char === '*') {
      const next = pattern[index + 1];
      if (next === '*') {
        const slash = pattern[index + 2];
        if (slash === '/') {
          expression += '(?:.*/)?';
          index += 2;
          continue;
        }
        expression += '.*';
        index += 1;
        continue;
      }
      expression += '[^/]*';
      continue;
    }
    if (char === '?') {
      expression += '[^/]';
      continue;
    }
    if ('\\^$+?.()|{}[]'.includes(char)) {
      expression += `\\${char}`;
      continue;
    }
    expression += char;
  }
  expression += '$';
  return new RegExp(expression);
}

export function matches(path, pattern) {
  return patternExpression(pattern).test(path);
}

export function matchesAny(path, patterns) {
  for (const pattern of patterns) {
    if (matches(path, pattern)) {
      return true;
    }
  }
  return false;
}

export function playwrightGroup(suite, id) {
  const group = groupByID(suite, id);
  if (group.runner !== 'playwright') {
    throw new Error(`E2E group ${id} uses ${group.runner}, not Playwright`);
  }
  return group;
}

export function allGroups(suite) {
  return suite.groups.map((group) => group.id);
}

export function ownersOf(suite, path) {
  return suite.groups.filter((group) => matchesAny(path, group.paths)).map((group) => group.id);
}

export function classify(suite, path) {
  if (matchesAny(path, suite.unitOnlyPaths)) {
    return { kind: UNIT_ONLY, groups: [] };
  }
  if (matchesAny(path, suite.fullRunPaths)) {
    return { kind: FULL, groups: allGroups(suite) };
  }
  const owners = ownersOf(suite, path);
  if (owners.length > 0) {
    return { kind: OWNED, groups: owners };
  }
  if (matchesAny(path, suite.productionRoots)) {
    return { kind: UNMAPPED, groups: allGroups(suite) };
  }
  return { kind: UNRELATED, groups: [] };
}

export function selectGroups(suite, files, options = {}) {
  const everything = allGroups(suite);
  if (options.all === true) {
    return { groups: everything, reason: 'every group was requested' };
  }
  if (files.length === 0 && options.push === true) {
    return { groups: everything, reason: 'a push with no diff runs every group' };
  }
  const selected = new Set([suite.smokeGroup]);
  for (const path of files) {
    const verdict = classify(suite, path);
    if (verdict.kind === FULL) {
      return { groups: everything, reason: `${path} is cross-cutting` };
    }
    if (verdict.kind === UNMAPPED) {
      return { groups: everything, reason: `${path} is production code that no group owns` };
    }
    for (const id of verdict.groups) {
      selected.add(id);
    }
  }
  return { groups: everything.filter((id) => selected.has(id)), reason: '' };
}

export const COMMIT = 'commit';
export const NIGHTLY = 'nightly';

export function knownTier(tier) {
  return tier === COMMIT || tier === NIGHTLY;
}

export function browsersFor(suite, tier) {
  if (tier === NIGHTLY) {
    return suite.nightlyBrowsers;
  }
  return suite.commitBrowsers;
}

export function tierGroups(suite, tier) {
  if (tier === NIGHTLY) {
    return allGroups(suite);
  }
  return suite.groups.filter((group) => group.tier === COMMIT).map((group) => group.id);
}

export function groupCost(group, browsers) {
  if (group.runner === 'playwright') {
    return group.observedMinutes * browsers.length;
  }
  if (group.runner === 'cluster-mode') {
    return group.observedMinutes + group.browserMinutes * browsers.length;
  }
  return group.observedMinutes;
}

export function tierCost(suite, tier) {
  const browsers = browsersFor(suite, tier);
  let total = 0;
  for (const id of tierGroups(suite, tier)) {
    total += groupCost(groupByID(suite, id), browsers);
  }
  return total;
}

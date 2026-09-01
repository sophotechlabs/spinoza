import { readFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
const e2e = resolve(here, '..');

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

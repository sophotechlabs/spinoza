import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { loadSuite, matchesAny } from './suite.mjs';

function argument(name) {
  const index = process.argv.indexOf(name);
  if (index === -1) {
    return '';
  }
  const value = process.argv[index + 1];
  if (value === undefined) {
    throw new Error(`${name} needs a value`);
  }
  return value;
}

function flag(name) {
  return process.argv.includes(name);
}

function changedFiles() {
  const supplied = argument('--files');
  if (supplied !== '') {
    return readFileSync(supplied, 'utf8')
      .split('\n')
      .map((path) => path.trim())
      .filter((path) => path !== '');
  }
  const base = argument('--base');
  const head = argument('--head');
  if (base === '' || head === '') {
    return [];
  }
  if (/^0+$/.test(base)) {
    return [];
  }
  return execFileSync('git', ['diff', '--name-only', `${base}...${head}`], {
    encoding: 'utf8',
  })
    .split('\n')
    .map((path) => path.trim())
    .filter((path) => path !== '');
}

function select(suite, files) {
  const selected = new Set([suite.smokeGroup]);
  if (flag('--all')) {
    return suite.groups.map((group) => group.id);
  }
  if (files.length === 0 && flag('--push')) {
    return suite.groups.map((group) => group.id);
  }
  for (const path of files) {
    if (matchesAny(path, suite.fullRunPaths)) {
      return suite.groups.map((group) => group.id);
    }
    let mapped = false;
    for (const group of suite.groups) {
      if (matchesAny(path, group.paths)) {
        selected.add(group.id);
        mapped = true;
      }
    }
    if (!mapped && matchesAny(path, suite.productionRoots)) {
      return suite.groups.map((group) => group.id);
    }
  }
  return suite.groups.filter((group) => selected.has(group.id)).map((group) => group.id);
}

const suite = loadSuite();
const files = changedFiles();
const ids = select(suite, files);
const include = ids.map((id) => {
  const group = suite.groups.find((candidate) => candidate.id === id);
  if (group === undefined) {
    throw new Error(`selected unknown group ${id}`);
  }
  return {
    group: group.id,
    runner: group.runner,
    profile: group.profile,
    timeout: group.timeoutMinutes,
  };
});
process.stderr.write(`selected ${ids.join(', ')} for ${String(files.length)} changed files\n`);
process.stdout.write(`${JSON.stringify({ include })}\n`);

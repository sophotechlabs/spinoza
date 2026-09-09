import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { groupByID, loadSuite, selectGroups } from './suite.mjs';

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

const suite = loadSuite();
const files = changedFiles();
const { groups: ids, reason } = selectGroups(suite, files, {
  all: flag('--all'),
  push: flag('--push'),
});
const include = ids.map((id) => {
  const group = groupByID(suite, id);
  return {
    group: group.id,
    runner: group.runner,
    profile: group.profile,
    timeout: group.timeoutMinutes,
  };
});
let why = '';
if (reason !== '') {
  why = ` because ${reason}`;
}
process.stderr.write(
  `selected ${ids.join(', ')} for ${String(files.length)} changed files${why}\n`,
);
process.stdout.write(`${JSON.stringify({ include, reason })}\n`);

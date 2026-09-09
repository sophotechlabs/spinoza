import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { relative, resolve } from 'node:path';
import {
  COMMIT,
  FULL,
  NIGHTLY,
  UNMAPPED,
  browsersFor,
  classify,
  knownTier,
  loadSuite,
  matches,
  matchesAny,
  tierCost,
} from './suite.mjs';

const e2e = resolve(import.meta.dirname, '..');
const repo = resolve(e2e, '..');
const suite = loadSuite();
const expectedGroups = [
  'foundation-security',
  'navigation-interaction',
  'visual-accessibility',
  'resources-live-tables',
  'inspect-compare-rbac-topology',
  'mutations-protection-history',
  'streams-terminals-forwards',
  'checks-issues-worklist',
  'helm',
  'gitops',
  'observability-traffic',
  'multicluster-fleet',
  'cluster-mode-auth',
  'outage',
  'resilience-capacity-soak',
  'mcp-cli',
  'distribution-desktop-install',
];

function trackedFiles() {
  return execFileSync('git', ['ls-files', '--cached', '--others', '--exclude-standard'], {
    cwd: repo,
    encoding: 'utf8',
  })
    .split('\n')
    .filter((path) => path !== '');
}

function fail(message) {
  process.stderr.write(`${message}\n`);
  process.exitCode = 1;
}

if (suite.schemaVersion !== 3) {
  fail(`suite schema is ${String(suite.schemaVersion)}, want 3`);
}

const knownBrowsers = new Set(['chromium', 'firefox', 'webkit']);
for (const [field, browsers] of [
  ['commitBrowsers', suite.commitBrowsers],
  ['nightlyBrowsers', suite.nightlyBrowsers],
]) {
  if (!Array.isArray(browsers) || browsers.length === 0) {
    fail(`suite ${field} is empty`);
    continue;
  }
  for (const browser of browsers) {
    if (!knownBrowsers.has(browser)) {
      fail(`suite ${field} names unknown browser ${browser}`);
    }
  }
}
for (const browser of suite.commitBrowsers ?? []) {
  if (!(suite.nightlyBrowsers ?? []).includes(browser)) {
    fail(`commitBrowsers has ${browser}, which nightlyBrowsers does not run`);
  }
}

const actualGroups = suite.groups.map((group) => group.id);
if (JSON.stringify(actualGroups) !== JSON.stringify(expectedGroups)) {
  fail(`suite groups are ${actualGroups.join(', ')}, want ${expectedGroups.join(', ')}`);
}

const knownRunners = new Set(['playwright', 'cluster-mode', 'mcp-cli', 'distribution']);
const claimed = new Map();
for (const group of suite.groups) {
  if (!knownRunners.has(group.runner)) {
    fail(`${group.id} uses unknown runner ${group.runner}`);
  }
  if (group.timeoutMinutes < 15 || group.timeoutMinutes > 180) {
    fail(`${group.id} timeout is outside 15..180 minutes`);
  }
  if (group.runner === 'playwright' && group.specs.length === 0) {
    fail(`${group.id} has no Playwright specs`);
  }
  if (!knownTier(group.tier)) {
    fail(`${group.id} declares tier ${String(group.tier)}, want ${COMMIT} or ${NIGHTLY}`);
  }
  if (!Number.isInteger(group.observedMinutes) || group.observedMinutes < 1) {
    fail(
      `${group.id} observedMinutes is ${String(group.observedMinutes)}, want a positive integer`,
    );
  }
  if (group.runner === 'cluster-mode') {
    if (!Number.isInteger(group.browserMinutes) || group.browserMinutes < 1) {
      fail(`${group.id} runs a browser fan-out and needs a positive browserMinutes`);
    }
  } else if (group.browserMinutes !== undefined) {
    fail(
      `${group.id} declares browserMinutes but its ${group.runner} runner has no browser fan-out`,
    );
  }
  for (const spec of group.specs) {
    const owners = claimed.get(spec);
    if (owners === undefined) {
      claimed.set(spec, [group.id]);
      continue;
    }
    owners.push(group.id);
  }
}

const files = trackedFiles();
for (const path of files) {
  const inside = relative('e2e', path);
  const inSpecRoot = suite.specRoots.some((root) => inside.startsWith(`${root}/`));
  if (!inSpecRoot || !path.endsWith('.spec.ts')) {
    continue;
  }
  const owners = claimed.get(inside);
  if (owners === undefined) {
    fail(`${path} belongs to no capability group`);
    continue;
  }
  if (owners.length !== 1) {
    fail(`${path} belongs to ${owners.join(', ')}`);
  }
}

for (const [spec, owners] of claimed.entries()) {
  const path = `e2e/${spec}`;
  if (!files.includes(path)) {
    fail(`${path} is claimed by ${owners.join(', ')} but is not tracked`);
  }
}

let crossCutting = 0;
for (const path of files) {
  const verdict = classify(suite, path);
  if (verdict.kind === UNMAPPED) {
    fail(`${path} has no E2E change mapping`);
    continue;
  }
  if (verdict.kind === FULL && matchesAny(path, suite.productionRoots)) {
    crossCutting += 1;
  }
}
const smoke = suite.groups.find((group) => group.id === suite.smokeGroup);
if (smoke === undefined) {
  fail(`smokeGroup ${suite.smokeGroup} is not a group`);
} else if (smoke.tier !== COMMIT) {
  fail(
    `smokeGroup ${suite.smokeGroup} is on the ${String(smoke.tier)} tier and would never run per commit`,
  );
}

let commitCost = 0;
if (typeof suite.commitBudgetMinutes !== 'number') {
  fail('suite has no commitBudgetMinutes');
} else {
  commitCost = tierCost(suite, COMMIT);
  if (commitCost > suite.commitBudgetMinutes) {
    fail(
      `the commit tier costs ${String(commitCost)} job-minutes on ` +
        `${browsersFor(suite, COMMIT).join(', ')}, budget is ` +
        `${String(suite.commitBudgetMinutes)}; move a group to ${NIGHTLY} or raise the budget`,
    );
  }
}

if (typeof suite.fullRunBudget !== 'number') {
  fail('suite has no fullRunBudget');
} else if (crossCutting > suite.fullRunBudget) {
  fail(
    `${String(crossCutting)} production files select every group, budget is ` +
      `${String(suite.fullRunBudget)}; give the new ones an owning group`,
  );
}

const patternSets = [
  ['productionRoots', suite.productionRoots],
  ['fullRunPaths', suite.fullRunPaths],
  ['unitOnlyPaths', suite.unitOnlyPaths],
  ...suite.groups.map((group) => [`${group.id}.paths`, group.paths]),
];
for (const [field, patterns] of patternSets) {
  for (const pattern of patterns) {
    if (!files.some((path) => matches(path, pattern))) {
      fail(`${field} pattern ${pattern} matches no tracked file`);
    }
  }
}

const forbidden = [
  { pattern: /test\.skip\s*\(/, label: 'test.skip' },
  { pattern: /test\.fixme\s*\(/, label: 'test.fixme' },
  { pattern: /mode:\s*['"]serial['"]/, label: 'serial mode' },
];
for (const path of files) {
  const inside = relative('e2e', path);
  const inSpecRoot = suite.specRoots.some((root) => inside.startsWith(`${root}/`));
  if (!inSpecRoot || !path.endsWith('.ts')) {
    continue;
  }
  const source = readFileSync(resolve(repo, path), 'utf8');
  for (const rule of forbidden) {
    if (rule.pattern.test(source)) {
      fail(`${path} contains forbidden ${rule.label}`);
    }
  }
}

if (process.exitCode === undefined) {
  let slack = '';
  if (crossCutting < suite.fullRunBudget) {
    slack = `; fullRunBudget can come down to ${String(crossCutting)}`;
  }
  let room = '';
  if (commitCost < suite.commitBudgetMinutes) {
    room = `, ${String(suite.commitBudgetMinutes - commitCost)} to spare`;
  }
  process.stdout.write(
    `validated ${String(suite.groups.length)} groups and ${String(claimed.size)} specs; ` +
      `${String(crossCutting)} of the production files select every group ` +
      `(budget ${String(suite.fullRunBudget)})${slack}\n` +
      `the ${COMMIT} tier costs ${String(commitCost)} job-minutes of ` +
      `${String(suite.commitBudgetMinutes)}${room}; ${NIGHTLY} costs ` +
      `${String(tierCost(suite, NIGHTLY))}\n`,
  );
}

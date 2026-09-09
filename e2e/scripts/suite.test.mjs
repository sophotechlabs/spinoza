import assert from 'node:assert/strict';
import { test } from 'node:test';
import {
  COMMIT,
  FULL,
  NIGHTLY,
  OWNED,
  UNIT_ONLY,
  UNMAPPED,
  UNRELATED,
  browsersFor,
  classify,
  groupCost,
  knownTier,
  loadSuite,
  matches,
  selectGroups,
  tierCost,
  tierGroups,
} from './suite.mjs';

const suite = {
  smokeGroup: 'smoke',
  productionRoots: ['*.go', 'internal/**', 'frontend/src/**'],
  fullRunPaths: ['internal/server/**', 'main.go', 'e2e/harness/**'],
  unitOnlyPaths: ['*_test.go', 'internal/**/*_test.go', 'internal/**/testdata/**'],
  groups: [
    { id: 'smoke', paths: ['internal/auth/**'] },
    { id: 'helm', paths: ['internal/helm/**', 'e2e/specs/helm.spec.ts'] },
    { id: 'streams', paths: ['internal/exec/**', 'internal/server/exec.go'] },
    { id: 'mcp', paths: ['test/integration/mcp_test.go'] },
  ],
};

test('a bare star stays inside one path segment', () => {
  assert.equal(matches('main.go', '*.go'), true);
  assert.equal(matches('internal/a.go', '*.go'), false);
  assert.equal(matches('components/PanelMount.tsx', 'components/Panel*.tsx'), true);
});

test('a leading double star is optional', () => {
  assert.equal(matches('a_test.go', '**/*_test.go'), true);
  assert.equal(matches('internal/x/a_test.go', '**/*_test.go'), true);
  assert.equal(matches('internal/x/a.go', '**/*_test.go'), false);
});

test('a trailing double star covers every depth', () => {
  assert.equal(matches('internal/server/a.go', 'internal/server/**'), true);
  assert.equal(matches('internal/server/deep/a.go', 'internal/server/**'), true);
  assert.equal(matches('internal/serverless/a.go', 'internal/server/**'), false);
});

test('a unit test is unit-only even inside a cross-cutting or owned tree', () => {
  assert.deepEqual(classify(suite, 'internal/server/exec_test.go'), {
    kind: UNIT_ONLY,
    groups: [],
  });
  assert.deepEqual(classify(suite, 'internal/helm/helm_test.go'), { kind: UNIT_ONLY, groups: [] });
  assert.deepEqual(classify(suite, 'flags_test.go'), { kind: UNIT_ONLY, groups: [] });
  assert.deepEqual(classify(suite, 'internal/helm/testdata/values.yaml'), {
    kind: UNIT_ONLY,
    groups: [],
  });
});

test('an integration test a group names belongs to that group', () => {
  assert.deepEqual(classify(suite, 'test/integration/mcp_test.go'), {
    kind: OWNED,
    groups: ['mcp'],
  });
});

test('cross-cutting wins over ownership', () => {
  assert.deepEqual(classify(suite, 'internal/server/exec.go'), {
    kind: FULL,
    groups: ['smoke', 'helm', 'streams', 'mcp'],
  });
});

test('production code no group owns is unmapped', () => {
  assert.deepEqual(classify(suite, 'internal/podcount/podcount.go'), {
    kind: UNMAPPED,
    groups: ['smoke', 'helm', 'streams', 'mcp'],
  });
});

test('everything else is unrelated', () => {
  assert.deepEqual(classify(suite, 'README.md'), { kind: UNRELATED, groups: [] });
  assert.deepEqual(classify(suite, 'test/integration/other_test.go'), {
    kind: UNRELATED,
    groups: [],
  });
});

test('no changes selects the smoke group only', () => {
  assert.deepEqual(selectGroups(suite, []), { groups: ['smoke'], reason: '' });
  assert.deepEqual(selectGroups(suite, ['README.md']), { groups: ['smoke'], reason: '' });
});

test('owned files select their groups in suite order plus smoke', () => {
  assert.deepEqual(selectGroups(suite, ['internal/exec/a.go', 'internal/helm/b.go']), {
    groups: ['smoke', 'helm', 'streams'],
    reason: '',
  });
});

test('a unit-only change selects the smoke group only', () => {
  assert.deepEqual(selectGroups(suite, ['internal/server/ws_test.go']), {
    groups: ['smoke'],
    reason: '',
  });
});

test('a cross-cutting file selects every group and says which file', () => {
  assert.deepEqual(selectGroups(suite, ['internal/helm/b.go', 'main.go']), {
    groups: ['smoke', 'helm', 'streams', 'mcp'],
    reason: 'main.go is cross-cutting',
  });
});

test('an unmapped production file selects every group and says so', () => {
  assert.deepEqual(selectGroups(suite, ['internal/podcount/podcount.go']), {
    groups: ['smoke', 'helm', 'streams', 'mcp'],
    reason: 'internal/podcount/podcount.go is production code that no group owns',
  });
});

test('--all and a diffless push run everything, a push with a diff selects', () => {
  assert.equal(selectGroups(suite, ['README.md'], { all: true }).groups.length, 4);
  assert.equal(selectGroups(suite, [], { push: true }).groups.length, 4);
  assert.deepEqual(selectGroups(suite, ['internal/helm/b.go'], { push: true }).groups, [
    'smoke',
    'helm',
  ]);
});

test('the checked-in suite maps an owned Go file to its group only', () => {
  const real = loadSuite();
  assert.deepEqual(selectGroups(real, ['internal/helm/helm.go']).groups, [
    'foundation-security',
    'helm',
  ]);
  assert.deepEqual(selectGroups(real, ['internal/server/helm.go']).groups, [
    'foundation-security',
    'helm',
  ]);
  assert.deepEqual(selectGroups(real, ['frontend/src/components/Helm.tsx']).groups, [
    'foundation-security',
    'helm',
  ]);
  assert.deepEqual(selectGroups(real, ['internal/server/ws_test.go']).groups, [
    'foundation-security',
  ]);
  assert.equal(selectGroups(real, ['internal/server/ws.go']).groups.length, real.groups.length);
});

const tiered = {
  commitBrowsers: ['chromium'],
  nightlyBrowsers: ['chromium', 'firefox', 'webkit'],
  groups: [
    { id: 'smoke', runner: 'playwright', tier: COMMIT, observedMinutes: 4 },
    { id: 'slow', runner: 'playwright', tier: NIGHTLY, observedMinutes: 10 },
    { id: 'cm', runner: 'cluster-mode', tier: COMMIT, observedMinutes: 10, browserMinutes: 7 },
    { id: 'cli', runner: 'mcp-cli', tier: COMMIT, observedMinutes: 4 },
  ],
};

test('only commit-tier groups are eligible per commit, every group at night', () => {
  assert.deepEqual(tierGroups(tiered, COMMIT), ['smoke', 'cm', 'cli']);
  assert.deepEqual(tierGroups(tiered, NIGHTLY), ['smoke', 'slow', 'cm', 'cli']);
});

test('each tier runs its own browsers', () => {
  assert.deepEqual(browsersFor(tiered, COMMIT), ['chromium']);
  assert.deepEqual(browsersFor(tiered, NIGHTLY), ['chromium', 'firefox', 'webkit']);
});

test('a Playwright group costs one job per browser', () => {
  const group = { runner: 'playwright', observedMinutes: 4 };
  assert.equal(groupCost(group, ['chromium']), 4);
  assert.equal(groupCost(group, ['chromium', 'firefox', 'webkit']), 12);
});

test('a cluster-mode group costs its own job plus one browser job each', () => {
  const group = { runner: 'cluster-mode', observedMinutes: 10, browserMinutes: 7 };
  assert.equal(groupCost(group, ['chromium']), 17);
  assert.equal(groupCost(group, ['chromium', 'firefox', 'webkit']), 31);
});

test('a group with no browser fan-out costs the same on either tier', () => {
  const group = { runner: 'mcp-cli', observedMinutes: 4 };
  assert.equal(groupCost(group, ['chromium']), 4);
  assert.equal(groupCost(group, ['chromium', 'firefox', 'webkit']), 4);
});

test('a tier costs the sum of the groups it runs', () => {
  assert.equal(tierCost(tiered, COMMIT), 25);
  assert.equal(tierCost(tiered, NIGHTLY), 77);
});

test('only the two declared tiers are known', () => {
  assert.equal(knownTier(COMMIT), true);
  assert.equal(knownTier(NIGHTLY), true);
  assert.equal(knownTier('weekly'), false);
  assert.equal(knownTier(undefined), false);
});

test('the checked-in commit tier fits its declared budget', () => {
  const checked = loadSuite();
  assert.ok(tierCost(checked, COMMIT) <= checked.commitBudgetMinutes);
  assert.ok(tierCost(checked, NIGHTLY) > tierCost(checked, COMMIT));
  assert.equal(
    checked.groups.every((group) => knownTier(group.tier)),
    true,
  );
});

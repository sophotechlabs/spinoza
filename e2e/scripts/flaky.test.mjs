import assert from 'node:assert/strict';
import { test } from 'node:test';
import { flakyTests, render, summarize } from './flaky.mjs';

function result(status, retry) {
  return {
    workerIndex: 0,
    parallelIndex: 0,
    status,
    duration: 1200,
    error: undefined,
    errors: [],
    stdout: [],
    stderr: [],
    retry,
    startTime: '2026-09-07T12:00:00.000Z',
    attachments: [],
    annotations: [],
  };
}

function spec(title, file, line, status, results) {
  return {
    tags: [],
    title,
    ok: status !== 'unexpected',
    id: `${file}-${String(line)}`,
    file,
    line,
    column: 1,
    tests: [
      {
        timeout: 90000,
        annotations: [],
        expectedStatus: 'passed',
        projectName: 'chromium',
        projectId: 'chromium',
        status,
        results,
      },
    ],
  };
}

const report = {
  config: {},
  errors: [],
  stats: {
    startTime: '2026-09-07T12:00:00.000Z',
    duration: 5000,
    expected: 2,
    unexpected: 1,
    flaky: 1,
    skipped: 0,
  },
  suites: [
    {
      title: 'specs/history.spec.ts',
      file: 'specs/history.spec.ts',
      column: 0,
      line: 0,
      specs: [
        spec('the view says what it is for', 'specs/history.spec.ts', 5, 'expected', [
          result('passed', 0),
        ]),
      ],
      suites: [
        {
          title: 'clearing',
          file: 'specs/history.spec.ts',
          column: 1,
          line: 20,
          specs: [
            spec('clearing the history', 'specs/history.spec.ts', 21, 'flaky', [
              result('failed', 0),
              result('passed', 1),
            ]),
          ],
        },
      ],
    },
    {
      title: 'specs/helm.spec.ts',
      file: 'specs/helm.spec.ts',
      column: 0,
      line: 0,
      specs: [
        spec('installs a chart', 'specs/helm.spec.ts', 9, 'expected', [result('passed', 0)]),
        spec('upgrades a release', 'specs/helm.spec.ts', 40, 'unexpected', [
          result('failed', 0),
          result('failed', 1),
        ]),
      ],
    },
  ],
};

test('a test that passed only on retry is listed with its describe trail and attempts', () => {
  assert.deepEqual(flakyTests(report), [
    {
      title: 'clearing › clearing the history',
      file: 'specs/history.spec.ts',
      line: 21,
      project: 'chromium',
      attempts: 2,
    },
  ]);
});

test('a failed test and a passed test are not flaky', () => {
  const summary = summarize(report);
  assert.equal(summary.expected, 2);
  assert.equal(summary.unexpected, 1);
  assert.equal(summary.flaky.length, 1);
});

test('a report whose count disagrees with its suites is refused', () => {
  const drifted = { ...report, stats: { ...report.stats, flaky: 3 } };
  assert.throws(() => summarize(drifted), /counts 3 flaky tests but 1/);
});

test('the summary renders a heading, the totals and one row per flaky test', () => {
  const text = render(summarize(report));
  assert.match(text, /^## Flaky tests: 1\n/);
  assert.match(text, /2 passed, 1 failed, 0 skipped, 1 passed only on retry\./);
  assert.match(
    text,
    /\| clearing › clearing the history \| specs\/history\.spec\.ts:21 \| chromium \| 2 \|/,
  );
});

test('a clean report renders no table', () => {
  const clean = { ...report, stats: { ...report.stats, flaky: 0 }, suites: [report.suites[1]] };
  const text = render(summarize(clean));
  assert.match(text, /^## Flaky tests: 0\n/);
  assert.doesNotMatch(text, /\| test \|/);
});

import assert from "node:assert/strict";
import { mkdirSync, mkdtempSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import {
  buildSummary,
  collectFlaky,
  collectMutation,
  minutesBetween,
  readJobs,
  render,
  statusOf,
  verdictOf,
} from "./nightly-report.mjs";

function scratch() {
  return mkdtempSync(join(tmpdir(), "nightly-report-"));
}

const jobs = [
  {
    name: "select",
    conclusion: "success",
    started_at: "2026-09-10T02:00:00Z",
    completed_at: "2026-09-10T02:00:06Z",
  },
  {
    name: "gitops / webkit",
    conclusion: "success",
    started_at: "2026-09-10T02:01:00Z",
    completed_at: "2026-09-10T02:08:00Z",
  },
  {
    name: "helm / chromium",
    conclusion: "failure",
    started_at: "2026-09-10T02:01:00Z",
    completed_at: "2026-09-10T02:04:30Z",
  },
];

test("infrastructure jobs are left out of the report", () => {
  const rows = readJobs(jobs);
  assert.deepEqual(
    rows.map((row) => row.name),
    ["gitops / webkit", "helm / chromium"],
  );
});

test("a job that never started counts as zero minutes rather than NaN", () => {
  assert.equal(minutesBetween(null, null), 0);
  assert.equal(minutesBetween("2026-09-10T02:00:00Z", null), 0);
  const rows = readJobs([{ name: "gitops / webkit", conclusion: null }]);
  assert.equal(rows[0].minutes, 0);
  assert.equal(rows[0].conclusion, "cancelled");
});

test("anything that did not succeed is a failure, cancellations included", () => {
  const summary = buildSummary({
    sha: "abc",
    runUrl: "u",
    trigger: "schedule",
    finishedAt: "2026-09-10T03:00:00Z",
    jobs: [
      {
        name: "a",
        conclusion: "success",
        started_at: "2026-09-10T02:00:00Z",
        completed_at: "2026-09-10T02:01:00Z",
      },
      {
        name: "b",
        conclusion: "cancelled",
        started_at: "2026-09-10T02:00:00Z",
        completed_at: "2026-09-10T02:01:00Z",
      },
    ],
    flaky: [],
    e2eCoverage: null,
    mutation: null,
  });
  assert.deepEqual(summary.failures, ["b"]);
  assert.equal(summary.jobs.failed, 1);
  assert.equal(verdictOf(summary), "nightly:regression");
});

test("a clean run is labelled green", () => {
  const summary = buildSummary({
    sha: "abc",
    runUrl: "u",
    trigger: "schedule",
    finishedAt: "2026-09-10T03:00:00Z",
    jobs: [
      {
        name: "a",
        conclusion: "success",
        started_at: "2026-09-10T02:00:00Z",
        completed_at: "2026-09-10T02:01:00Z",
      },
    ],
    flaky: [],
    e2eCoverage: null,
    mutation: null,
  });
  assert.equal(verdictOf(summary), "nightly:green");
});

test("flaky tests are gathered from every group report", () => {
  const dir = scratch();
  const report = (file, title) => ({
    stats: { expected: 1, unexpected: 0, skipped: 0, flaky: 1 },
    suites: [
      {
        title: file,
        specs: [
          {
            title,
            file,
            line: 12,
            tests: [
              { status: "flaky", projectName: "webkit", results: [{}, {}] },
            ],
          },
        ],
        suites: [],
      },
    ],
  });
  mkdirSync(join(dir, "e2e-report-gitops-webkit"), { recursive: true });
  mkdirSync(join(dir, "e2e-report-helm-firefox"), { recursive: true });
  writeFileSync(
    join(dir, "e2e-report-gitops-webkit", "report.json"),
    JSON.stringify(report("specs/gitops.spec.ts", "it syncs")),
  );
  writeFileSync(
    join(dir, "e2e-report-helm-firefox", "report.json"),
    JSON.stringify(report("specs/helm.spec.ts", "it installs")),
  );
  const flaky = collectFlaky(dir);
  assert.equal(flaky.length, 2);
  assert.deepEqual(
    flaky.map((entry) => entry.file),
    ["specs/gitops.spec.ts", "specs/helm.spec.ts"],
  );
});

test("the mutation score is the killed share of the shards that reported", () => {
  const dir = scratch();
  writeFileSync(
    join(dir, "a.json"),
    JSON.stringify({ mutants_killed: 30, mutants_lived: 10 }),
  );
  writeFileSync(
    join(dir, "b.json"),
    JSON.stringify({ mutants_killed: 50, mutants_lived: 10 }),
  );
  const mutation = collectMutation(dir);
  assert.deepEqual(mutation, { shards: 2, killed: 80, lived: 20, score: 80 });
});

test("a report with no mutation shards says so instead of claiming zero", () => {
  assert.equal(collectMutation(join(scratch(), "absent")), null);
  assert.equal(collectMutation(scratch()), null);
});

const summary = buildSummary({
  sha: "0736554aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  runUrl: "https://github.com/sophotechlabs/spinoza/actions/runs/1",
  trigger: "schedule",
  finishedAt: "2026-09-10T03:00:00Z",
  jobs,
  flaky: [
    {
      title: "it syncs",
      file: "specs/gitops.spec.ts",
      line: 12,
      project: "webkit",
      attempts: 2,
    },
  ],
  e2eCoverage: 71.2,
  mutation: { shards: 23, killed: 800, lived: 200, score: 80 },
});

test("the report names the run, the commit and the verdict", () => {
  const text = render(summary, null);
  assert.match(text, /# Nightly 2026-09-10 — regression/);
  assert.match(text, /Commit `0736554aaaaa`/);
  assert.match(text, /actions\/runs\/1/);
});

test("every number carries its delta against the previous run", () => {
  const previous = {
    jobs: { total: 2, failed: 0, minutes: 9 },
    failures: [],
    flaky: [],
    e2eCoverage: 70.2,
    mutation: { score: 78 },
  };
  const text = render(summary, previous);
  assert.match(text, /71\.2% \(\+1\)/);
  assert.match(text, /80% \(\+2\)/);
  assert.match(text, /1 \(\+1\) \| passed only on retry/);
});

test("an unchanged number says so rather than showing a bare zero", () => {
  const previous = {
    jobs: { minutes: 10.5 },
    failures: [],
    flaky: [],
    e2eCoverage: 71.2,
  };
  assert.match(render(summary, previous), /71\.2% \(no change\)/);
});

test("a first run has no baseline and shows the bare numbers", () => {
  const text = render(summary, null);
  assert.match(text, /\| 71\.2% \|/);
  assert.doesNotMatch(text, /\(\+/);
});

test("a run that did not measure something says n/a instead of guessing", () => {
  const bare = buildSummary({
    sha: "abc123abc123",
    runUrl: "u",
    trigger: "push",
    finishedAt: "2026-09-10T03:00:00Z",
    jobs,
    flaky: [],
    e2eCoverage: null,
    mutation: null,
  });
  const text = render(bare, null);
  assert.match(text, /\| e2e coverage \| n\/a \|/);
  assert.match(text, /not run/);
});

test("new failures and fixes are called out against the previous run", () => {
  const previous = {
    jobs: { minutes: 9 },
    failures: ["gitops / webkit"],
    flaky: [],
  };
  const text = render(summary, previous);
  assert.match(text, /## New failures\n\n- helm \/ chromium/);
  assert.match(text, /## Fixed since the last run\n\n- gitops \/ webkit/);
});

test("a green run lists no failure sections", () => {
  const green = buildSummary({
    sha: "abc123abc123",
    runUrl: "u",
    trigger: "schedule",
    finishedAt: "2026-09-10T03:00:00Z",
    jobs: [
      {
        name: "a",
        conclusion: "success",
        started_at: "2026-09-10T02:00:00Z",
        completed_at: "2026-09-10T02:01:00Z",
      },
    ],
    flaky: [],
    e2eCoverage: 71.2,
    mutation: null,
  });
  const text = render(green, null);
  assert.match(text, /— green/);
  assert.doesNotMatch(text, /## Failing/);
  assert.doesNotMatch(text, /## New failures/);
});

test("a run notifies only when the set of failing jobs changed", () => {
  const failing = buildSummary({
    sha: "abc123abc123",
    runUrl: "u",
    trigger: "schedule",
    finishedAt: "2026-09-10T03:00:00Z",
    jobs,
    flaky: [],
    e2eCoverage: null,
    mutation: null,
  });
  assert.deepEqual(statusOf(failing, null), {
    verdict: "nightly:regression",
    notify: true,
  });
  assert.deepEqual(statusOf(failing, { failures: ["helm / chromium"] }), {
    verdict: "nightly:regression",
    notify: false,
  });
  assert.deepEqual(statusOf(failing, { failures: ["gitops / webkit"] }), {
    verdict: "nightly:regression",
    notify: true,
  });
});

test("a green run never notifies, whatever failed before", () => {
  const green = buildSummary({
    sha: "abc123abc123",
    runUrl: "u",
    trigger: "schedule",
    finishedAt: "2026-09-10T03:00:00Z",
    jobs: [
      {
        name: "a",
        conclusion: "success",
        started_at: "2026-09-10T02:00:00Z",
        completed_at: "2026-09-10T02:01:00Z",
      },
    ],
    flaky: [],
    e2eCoverage: null,
    mutation: null,
  });
  assert.deepEqual(statusOf(green, { failures: ["helm / chromium"] }), {
    verdict: "nightly:green",
    notify: false,
  });
});

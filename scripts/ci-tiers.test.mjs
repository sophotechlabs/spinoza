import assert from "node:assert/strict";
import { test } from "node:test";
import { TIERS, checkTiers } from "./ci-tiers.mjs";

const manifest = {
  budgetMinutes: 100,
  releaseOnlyPaths: ["CHANGELOG.md"],
  reportPath: "docs/ci/nightly/**",
  workflows: {
    "go.yaml": { tier: "commit", jobMinutes: 20 },
    "nightly.yaml": { tier: "nightly", jobMinutes: 0 },
  },
};

const commitOn = {
  push: { branches: ["main"], "paths-ignore": ["docs/ci/nightly/**"] },
  pull_request: { "paths-ignore": ["CHANGELOG.md", "docs/ci/nightly/**"] },
};

const nightly = {
  on: { push: { branches: ["main"] }, schedule: [{ cron: "37 2 * * *" }] },
  jobs: {
    guard: {},
    e2e: { needs: "guard" },
    report: { needs: ["guard", "e2e"] },
  },
};

function workflows(overrides = {}) {
  return { "go.yaml": { on: commitOn }, "nightly.yaml": nightly, ...overrides };
}

test("a clean pair of workflows passes and reports the budget", () => {
  const { failures, budget } = checkTiers(manifest, workflows(), 30);
  assert.deepEqual(failures, []);
  assert.equal(budget, 50);
});

test("a workflow missing from the manifest is refused", () => {
  const { failures } = checkTiers(
    manifest,
    workflows({ "new.yaml": { on: commitOn } }),
    30,
  );
  assert.deepEqual(failures, [
    "new.yaml is not in .github/ci-tiers.json; declare its tier",
  ]);
});

test("a manifest entry with no workflow is refused", () => {
  const only = {
    ...manifest,
    workflows: {
      ...manifest.workflows,
      "gone.yaml": { tier: "commit", jobMinutes: 1 },
    },
  };
  const { failures } = checkTiers(only, workflows(), 30);
  assert.deepEqual(failures, [
    ".github/ci-tiers.json declares gone.yaml, which does not exist",
  ]);
});

test("a commit-tier workflow that reruns for the nightly report is refused", () => {
  const bare = {
    on: {
      push: { branches: ["main"] },
      pull_request: { "paths-ignore": ["CHANGELOG.md"] },
    },
  };
  const { failures } = checkTiers(manifest, workflows({ "go.yaml": bare }), 30);
  assert.deepEqual(failures, [
    "go.yaml reruns on push for a docs/ci/nightly/** change",
    "go.yaml reruns on pull_request for docs/ci/nightly/**",
  ]);
});

test("a commit-tier workflow that reruns for a release-only change is refused", () => {
  const bare = {
    on: {
      push: { branches: ["main"], "paths-ignore": ["docs/ci/nightly/**"] },
      pull_request: { "paths-ignore": ["docs/ci/nightly/**"] },
    },
  };
  const { failures } = checkTiers(manifest, workflows({ "go.yaml": bare }), 30);
  assert.deepEqual(failures, [
    "go.yaml reruns on pull_request for CHANGELOG.md",
  ]);
});

test("a commit-tier workflow that skips pull requests is refused", () => {
  const bare = {
    on: {
      push: { branches: ["main"], "paths-ignore": ["docs/ci/nightly/**"] },
    },
  };
  const { failures } = checkTiers(manifest, workflows({ "go.yaml": bare }), 30);
  assert.ok(
    failures.includes(
      "go.yaml is on the commit tier but does not run on both push and pull_request",
    ),
  );
});

test("a nightly job that does not wait for the guard would run on every push", () => {
  const leaky = { ...nightly, jobs: { guard: {}, e2e: {} } };
  const { failures } = checkTiers(
    manifest,
    workflows({ "nightly.yaml": leaky }),
    30,
  );
  assert.deepEqual(failures, [
    "nightly.yaml job e2e does not wait for the guard and would run on every push",
  ]);
});

test("a nightly workflow with no guard is refused", () => {
  const guardless = { ...nightly, jobs: { e2e: {} } };
  const { failures } = checkTiers(
    manifest,
    workflows({ "nightly.yaml": guardless }),
    30,
  );
  assert.ok(
    failures.includes(
      "nightly.yaml is on the nightly tier and needs a guard job",
    ),
  );
});

test("a called or weekly workflow that still runs per commit is refused", () => {
  for (const tier of ["called", "weekly"]) {
    const scoped = {
      ...manifest,
      workflows: {
        "go.yaml": { tier, jobMinutes: 5 },
        "nightly.yaml": { tier: "nightly", jobMinutes: 0 },
      },
    };
    const { failures } = checkTiers(scoped, workflows(), 30);
    assert.ok(
      failures.includes(
        `go.yaml is on the ${tier} tier but still runs on push`,
      ),
    );
    assert.ok(
      failures.includes(
        `go.yaml is on the ${tier} tier but still runs on pull_request`,
      ),
    );
  }
});

test("a called workflow that is not reusable is refused", () => {
  const scoped = {
    ...manifest,
    workflows: {
      "go.yaml": { tier: "called", jobMinutes: 5 },
      "nightly.yaml": { tier: "nightly", jobMinutes: 0 },
    },
  };
  const { failures } = checkTiers(
    scoped,
    workflows({ "go.yaml": { on: { workflow_dispatch: null } } }),
    30,
  );
  assert.deepEqual(failures, [
    "go.yaml is on the called tier but is not reusable",
  ]);
});

test("a scoped workflow has to name the paths it watches", () => {
  const scoped = {
    ...manifest,
    workflows: {
      "go.yaml": { tier: "scoped", jobMinutes: 5 },
      "nightly.yaml": { tier: "nightly", jobMinutes: 0 },
    },
  };
  const open = { on: { pull_request: {} } };
  assert.deepEqual(
    checkTiers(scoped, workflows({ "go.yaml": open }), 30).failures,
    ["go.yaml is on the scoped tier but names no paths"],
  );
  const named = { on: { pull_request: { paths: ["install.sh"] } } };
  assert.deepEqual(
    checkTiers(scoped, workflows({ "go.yaml": named }), 30).failures,
    [],
  );
});

test("a main-tier workflow must not run on every pull request", () => {
  const scoped = {
    ...manifest,
    workflows: {
      "go.yaml": { tier: "main", jobMinutes: 5 },
      "nightly.yaml": { tier: "nightly", jobMinutes: 0 },
    },
  };
  const { failures } = checkTiers(scoped, workflows(), 30);
  assert.deepEqual(failures, [
    "go.yaml is on the main tier but also runs on every pull request",
  ]);
});

test("the budget counts the suite alongside the commit and main tiers", () => {
  const { failures } = checkTiers(manifest, workflows(), 81);
  assert.deepEqual(failures, [
    "the per-commit tiers cost 101 job-minutes, budget is 100; " +
      "move a workflow off the commit tier or raise the budget",
  ]);
});

test("an unknown tier is refused before anything else is checked", () => {
  const scoped = {
    ...manifest,
    workflows: {
      "go.yaml": { tier: "someday", jobMinutes: 5 },
      "nightly.yaml": { tier: "nightly", jobMinutes: 0 },
    },
  };
  const { failures } = checkTiers(scoped, workflows(), 30);
  assert.deepEqual(failures, ["go.yaml declares unknown tier someday"]);
  assert.deepEqual(TIERS, [
    "commit",
    "main",
    "nightly",
    "weekly",
    "called",
    "scoped",
  ]);
});

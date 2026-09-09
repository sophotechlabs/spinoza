import { readFileSync, readdirSync } from "node:fs";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { COMMIT, tierCost } from "../e2e/scripts/suite.mjs";

export const TIERS = [
  "commit",
  "main",
  "nightly",
  "weekly",
  "called",
  "scoped",
];

function triggers(workflow) {
  return workflow.on ?? workflow[true] ?? {};
}

function ignored(trigger) {
  if (trigger === null || trigger === undefined) {
    return [];
  }
  return trigger["paths-ignore"] ?? [];
}

function has(workflow, name) {
  return Object.hasOwn(triggers(workflow), name);
}

export function checkTiers(manifest, workflows, suiteCost) {
  const failures = [];
  const declared = Object.keys(manifest.workflows);
  const present = Object.keys(workflows);
  for (const name of present) {
    if (!declared.includes(name)) {
      failures.push(
        `${name} is not in .github/ci-tiers.json; declare its tier`,
      );
    }
  }
  for (const name of declared) {
    if (!present.includes(name)) {
      failures.push(
        `.github/ci-tiers.json declares ${name}, which does not exist`,
      );
    }
  }

  const wanted = [...manifest.releaseOnlyPaths, manifest.reportPath];
  let budget = 0;
  for (const [name, entry] of Object.entries(manifest.workflows)) {
    const workflow = workflows[name];
    if (workflow === undefined) {
      continue;
    }
    if (!TIERS.includes(entry.tier)) {
      failures.push(`${name} declares unknown tier ${String(entry.tier)}`);
      continue;
    }
    if (entry.tier === "commit" || entry.tier === "main") {
      budget += entry.jobMinutes;
    }
    const on = triggers(workflow);
    if (entry.tier === "commit") {
      if (!has(workflow, "push") || !has(workflow, "pull_request")) {
        failures.push(
          `${name} is on the commit tier but does not run on both push and pull_request`,
        );
      }
      for (const scope of ["push", "pull_request"]) {
        const list = ignored(on[scope]);
        const missing = wanted.filter((path) => !list.includes(path));
        if (scope === "push" && !list.includes(manifest.reportPath)) {
          failures.push(
            `${name} reruns on ${scope} for a ${manifest.reportPath} change`,
          );
          continue;
        }
        if (scope === "pull_request" && missing.length > 0) {
          failures.push(`${name} reruns on ${scope} for ${missing.join(", ")}`);
        }
      }
    }
    if (entry.tier === "main") {
      if (!has(workflow, "push")) {
        failures.push(`${name} is on the main tier but does not run on push`);
      }
      if (has(workflow, "pull_request")) {
        failures.push(
          `${name} is on the main tier but also runs on every pull request`,
        );
      }
    }
    if (entry.tier === "called" || entry.tier === "weekly") {
      for (const scope of ["push", "pull_request"]) {
        if (has(workflow, scope)) {
          failures.push(
            `${name} is on the ${entry.tier} tier but still runs on ${scope}`,
          );
        }
      }
    }
    if (entry.tier === "called" && !has(workflow, "workflow_call")) {
      failures.push(`${name} is on the called tier but is not reusable`);
    }
    if (entry.tier === "scoped") {
      if (!has(workflow, "pull_request")) {
        failures.push(
          `${name} is on the scoped tier but does not run on pull_request`,
        );
      } else if ((on.pull_request?.paths ?? []).length === 0) {
        failures.push(`${name} is on the scoped tier but names no paths`);
      }
    }
    if (entry.tier === "nightly") {
      const jobs = workflow.jobs ?? {};
      if (jobs.guard === undefined) {
        failures.push(`${name} is on the nightly tier and needs a guard job`);
      }
      for (const [job, body] of Object.entries(jobs)) {
        if (job === "guard") {
          continue;
        }
        const needs = [body.needs ?? []].flat();
        if (!needs.includes("guard")) {
          failures.push(
            `${name} job ${job} does not wait for the guard and would run on every push`,
          );
        }
      }
      if (has(workflow, "pull_request")) {
        failures.push(
          `${name} is on the nightly tier but runs on every pull request`,
        );
      }
    }
  }

  budget += suiteCost;
  if (budget > manifest.budgetMinutes) {
    failures.push(
      `the per-commit tiers cost ${String(budget)} job-minutes, budget is ` +
        `${String(manifest.budgetMinutes)}; move a workflow off the commit tier or raise the budget`,
    );
  }
  return { failures, budget };
}

function main() {
  const dir = process.argv[2];
  if (dir === undefined) {
    throw new Error("give the directory of workflows converted to JSON");
  }
  const root = resolve(import.meta.dirname, "..");
  const manifest = JSON.parse(
    readFileSync(join(root, ".github/ci-tiers.json"), "utf8"),
  );
  const workflows = {};
  for (const entry of readdirSync(dir)) {
    if (!entry.endsWith(".json")) {
      continue;
    }
    workflows[entry.replace(/\.json$/, ".yaml")] = JSON.parse(
      readFileSync(join(dir, entry), "utf8"),
    );
  }
  const suite = JSON.parse(readFileSync(join(root, "e2e/suite.json"), "utf8"));
  const { failures, budget } = checkTiers(
    manifest,
    workflows,
    tierCost(suite, COMMIT),
  );
  for (const failure of failures) {
    process.stderr.write(`${failure}\n`);
  }
  if (failures.length > 0) {
    process.exitCode = 1;
    return;
  }
  process.stdout.write(
    `checked ${String(Object.keys(manifest.workflows).length)} workflows; ` +
      `the per-commit tiers cost ${String(budget)} job-minutes of ` +
      `${String(manifest.budgetMinutes)}, ${String(manifest.budgetMinutes - budget)} to spare\n`,
  );
}

if (
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}

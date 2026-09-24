import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { summarize } from "../e2e/scripts/flaky.mjs";

const INFRASTRUCTURE = new Set([
  "select",
  "suite-contract",
  "e2e-coverage",
  "guard",
  "published",
  "report",
]);

export function minutesBetween(started, completed) {
  if (started === null || completed === null) {
    return 0;
  }
  const span = Date.parse(completed) - Date.parse(started);
  return Math.round((span / 60000) * 10) / 10;
}

export function readJobs(jobs) {
  const rows = [];
  for (const job of jobs) {
    if (INFRASTRUCTURE.has(job.name)) {
      continue;
    }
    rows.push({
      name: job.name,
      conclusion: job.conclusion ?? "cancelled",
      minutes: minutesBetween(job.started_at ?? null, job.completed_at ?? null),
    });
  }
  rows.sort((left, right) => left.name.localeCompare(right.name));
  return rows;
}

function filesUnder(dir, match) {
  const found = [];
  if (!existsSync(dir)) {
    return found;
  }
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      found.push(...filesUnder(path, match));
      continue;
    }
    if (match(entry)) {
      found.push(path);
    }
  }
  return found;
}

function jsonUnder(dir) {
  return filesUnder(dir, (entry) => entry.endsWith(".json"));
}

export function collectFlaky(dir) {
  const flaky = [];
  for (const path of filesUnder(dir, (entry) => entry === "report.json")) {
    const summary = summarize(JSON.parse(readFileSync(path, "utf8")));
    flaky.push(...summary.flaky);
  }
  flaky.sort((left, right) =>
    `${left.file}${left.title}`.localeCompare(`${right.file}${right.title}`),
  );
  return flaky;
}

export function collectMutation(dir) {
  if (!existsSync(dir)) {
    return null;
  }
  let killed = 0;
  let lived = 0;
  let uncovered = 0;
  let uncoveredKnown = true;
  let found = 0;
  for (const path of jsonUnder(dir)) {
    const report = JSON.parse(readFileSync(path, "utf8"));
    if (
      typeof report.mutants_killed !== "number" ||
      typeof report.mutants_lived !== "number"
    ) {
      continue;
    }
    killed += report.mutants_killed;
    lived += report.mutants_lived;
    if (
      Number.isInteger(report.mutants_not_covered) &&
      report.mutants_not_covered >= 0
    ) {
      uncovered += report.mutants_not_covered;
    } else {
      uncoveredKnown = false;
    }
    found += 1;
  }
  if (found === 0) {
    return null;
  }
  const total = killed + lived;
  let score = 0;
  if (total > 0) {
    score = Math.round((killed / total) * 1000) / 10;
  }
  if (!uncoveredKnown) {
    uncovered = null;
  }
  return { shards: found, killed, lived, uncovered, score };
}

export function collectSystemCoverage(dir) {
  const reports = [];
  const suites = new Set();
  for (const path of filesUnder(dir, (entry) => entry === "summary.json")) {
    const report = JSON.parse(readFileSync(path, "utf8"));
    if (
      typeof report.suite !== "string" ||
      !/^[a-z][a-z0-9-]+$/.test(report.suite) ||
      !Number.isSafeInteger(report.covered) ||
      !Number.isSafeInteger(report.total) ||
      report.covered < 0 ||
      report.total <= 0 ||
      report.covered > report.total
    ) {
      throw new Error(`invalid system coverage report: ${path}`);
    }
    if (suites.has(report.suite)) {
      throw new Error(`duplicate system coverage suite: ${report.suite}`);
    }
    suites.add(report.suite);
    reports.push({
      ...report,
      percent: Math.round((1000 * report.covered) / report.total) / 10,
    });
  }
  return reports.sort((left, right) => left.suite.localeCompare(right.suite));
}

export function buildSummary(input) {
  const rows = readJobs(input.jobs);
  const failures = rows
    .filter((row) => row.conclusion !== "success")
    .map((row) => row.name);
  const minutes =
    Math.round(rows.reduce((total, row) => total + row.minutes, 0) * 10) / 10;
  let systemCoverage = input.systemCoverage;
  if (systemCoverage === undefined || systemCoverage === null) {
    systemCoverage = [];
  }
  return {
    sha: input.sha,
    runUrl: input.runUrl,
    trigger: input.trigger,
    finishedAt: input.finishedAt,
    jobs: { total: rows.length, failed: failures.length, minutes },
    groups: rows,
    failures,
    flaky: input.flaky,
    e2eCoverage: input.e2eCoverage,
    mutation: input.mutation,
    systemCoverage,
  };
}

function delta(current, previous, unit = "") {
  if (current === null || current === undefined) {
    return "n/a";
  }
  const shown = `${String(current)}${unit}`;
  if (previous === null || previous === undefined) {
    return shown;
  }
  const change = Math.round((current - previous) * 10) / 10;
  if (change === 0) {
    return `${shown} (no change)`;
  }
  if (change > 0) {
    return `${shown} (+${String(change)})`;
  }
  return `${shown} (${String(change)})`;
}

export function render(summary, previous) {
  const older = previous ?? {};
  const verdict = summary.failures.length === 0 ? "green" : "regression";
  const newly = summary.failures.filter(
    (name) => !(older.failures ?? []).includes(name),
  );
  const fixed = (older.failures ?? []).filter(
    (name) => !summary.failures.includes(name),
  );
  const lines = [
    `# Nightly ${summary.finishedAt.slice(0, 10)} — ${verdict}`,
    "",
    `Commit \`${summary.sha.slice(0, 12)}\`, triggered by ${summary.trigger}. [Run](${summary.runUrl})`,
    "",
    "| | this run | |",
    "|---|---|---|",
    `| jobs | ${String(summary.jobs.total)} | ${String(summary.jobs.failed)} failed |`,
    `| job-minutes | ${delta(summary.jobs.minutes, older.jobs?.minutes)} | |`,
    `| e2e coverage | ${delta(summary.e2eCoverage, older.e2eCoverage, "%")} | |`,
    `| mutation score | ${delta(summary.mutation?.score ?? null, older.mutation?.score ?? null, "%")} | ${
      summary.mutation === null || summary.mutation === undefined
        ? "not run"
        : `${String(summary.mutation.killed)} killed, ${String(summary.mutation.lived)} lived`
    } |`,
    `| flaky | ${delta(summary.flaky.length, older.flaky?.length)} | passed only on retry |`,
  ];
  const uncovered = summary.mutation?.uncovered;
  if (uncovered === null || uncovered === undefined) {
    lines.push(
      "| uncovered mutants | n/a | absent from one or more package reports |",
    );
  } else {
    lines.push(
      `| uncovered mutants | ${delta(uncovered, older.mutation?.uncovered)} | records across reported build variants |`,
    );
  }
  lines.push(
    "",
    "Mutation score is killed / (killed + lived). Uncovered mutants are excluded from that score; some are uninstrumented constant expressions.",
  );
  const systemCoverage = summary.systemCoverage;
  if (
    systemCoverage !== undefined &&
    systemCoverage !== null &&
    systemCoverage.length > 0
  ) {
    lines.push(
      "",
      "## Separate system coverage",
      "",
      "These suites retain their own Go statement denominators. The browser E2E coverage above is unchanged by these profiles.",
      "",
      "| suite | covered / total statements | coverage |",
      "|---|---|---|",
    );
    for (const report of systemCoverage) {
      lines.push(
        `| ${report.suite} | ${report.covered} / ${report.total} | ${report.percent}% |`,
      );
    }
  }
  if (newly.length > 0) {
    lines.push("", "## New failures", "");
    for (const name of newly) {
      lines.push(`- ${name}`);
    }
  }
  if (fixed.length > 0) {
    lines.push("", "## Fixed since the last run", "");
    for (const name of fixed) {
      lines.push(`- ${name}`);
    }
  }
  if (summary.failures.length > 0) {
    lines.push("", "## Failing", "");
    for (const name of summary.failures) {
      lines.push(`- ${name}`);
    }
  }
  if (summary.flaky.length > 0) {
    lines.push(
      "",
      "## Flaky",
      "",
      "| test | file | project | attempts |",
      "|---|---|---|---|",
    );
    for (const test of summary.flaky) {
      lines.push(
        `| ${test.title} | ${test.file}:${String(test.line)} | ${test.project} | ${String(test.attempts)} |`,
      );
    }
  }
  lines.push("", "## Jobs", "", "| job | result | minutes |", "|---|---|---|");
  for (const row of summary.groups) {
    lines.push(`| ${row.name} | ${row.conclusion} | ${String(row.minutes)} |`);
  }
  return `${lines.join("\n")}\n`;
}

export function verdictOf(summary) {
  if (summary.failures.length === 0) {
    return "nightly:green";
  }
  return "nightly:regression";
}

export function statusOf(summary, previous) {
  const before = (previous ?? {}).failures ?? [];
  const now = summary.failures;
  const changed =
    before.length !== now.length || now.some((name) => !before.includes(name));
  return { verdict: verdictOf(summary), notify: changed && now.length > 0 };
}

function argument(name, fallback = "") {
  const index = process.argv.indexOf(name);
  if (index === -1) {
    return fallback;
  }
  const value = process.argv[index + 1];
  if (value === undefined) {
    throw new Error(`${name} needs a value`);
  }
  return value;
}

function main() {
  const out = argument("--out", "docs/ci/nightly");
  const jobsPath = argument("--jobs");
  if (jobsPath === "") {
    throw new Error("--jobs needs the run job list");
  }
  const raw = JSON.parse(readFileSync(jobsPath, "utf8"));
  const jobs = Array.isArray(raw) ? raw : raw.jobs;
  const reports = argument("--reports");
  const mutation = argument("--mutation");
  const coverage = argument("--coverage");
  const previousPath = argument("--previous", join(out, "latest.json"));
  let previous = null;
  if (existsSync(previousPath)) {
    previous = JSON.parse(readFileSync(previousPath, "utf8"));
  }
  const summary = buildSummary({
    sha: argument("--sha"),
    runUrl: argument("--run-url"),
    trigger: argument("--trigger", "schedule"),
    finishedAt: new Date().toISOString(),
    jobs,
    flaky: reports === "" ? [] : collectFlaky(reports),
    e2eCoverage: coverage === "" ? null : Number(coverage),
    mutation: mutation === "" ? null : collectMutation(mutation),
    systemCoverage: collectSystemCoverage(argument("--system-coverage")),
  });
  mkdirSync(resolve(out), { recursive: true });
  writeFileSync(
    join(out, "latest.json"),
    `${JSON.stringify(summary, null, 2)}\n`,
  );
  writeFileSync(join(out, "report.md"), render(summary, previous));
  const status = statusOf(summary, previous);
  const statusPath = argument("--status");
  if (statusPath !== "") {
    writeFileSync(statusPath, `${JSON.stringify(status)}\n`);
  }
  process.stdout.write(`${status.verdict}\n`);
}

if (
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}

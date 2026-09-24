import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { pathToFileURL } from "node:url";

export function summarizeProfile(text, suite) {
  if (!/^[a-z][a-z0-9-]+$/.test(suite)) {
    throw new Error("coverage suite must be a stable lowercase name");
  }
  const lines = text.trim().split(/\r?\n/);
  if (!/^mode: (set|count|atomic)$/.test(lines.shift())) {
    throw new Error("coverage profile has no valid mode");
  }
  const blocks = new Map();
  for (const line of lines) {
    const match = /^(\S+:\d+\.\d+,\d+\.\d+) (\d+) (\d+)$/.exec(line);
    if (match === null) {
      throw new Error(`invalid coverage block: ${line}`);
    }
    const [, position, size, executions] = match;
    const statements = Number(size);
    const count = Number(executions);
    if (!Number.isSafeInteger(statements) || !Number.isSafeInteger(count)) {
      throw new Error(`invalid coverage count: ${position}`);
    }
    const previous = blocks.get(position);
    if (previous !== undefined && previous.statements !== statements) {
      throw new Error(`conflicting statement counts: ${position}`);
    }
    let hit = count > 0;
    if (previous !== undefined && previous.hit) {
      hit = true;
    }
    blocks.set(position, { statements, hit });
  }
  let covered = 0;
  let total = 0;
  for (const block of blocks.values()) {
    total += block.statements;
    if (block.hit) {
      covered += block.statements;
    }
  }
  if (total === 0) {
    throw new Error("coverage profile contains no instrumented statements");
  }
  return {
    suite,
    covered,
    total,
    percent: Math.round((1000 * covered) / total) / 10,
  };
}

function main() {
  const [profile, suite, output] = process.argv.slice(2);
  if (profile === undefined || suite === undefined || output === undefined) {
    throw new Error("usage: system-coverage.mjs PROFILE SUITE OUTPUT");
  }
  const report = summarizeProfile(readFileSync(profile, "utf8"), suite);
  mkdirSync(dirname(output), { recursive: true });
  writeFileSync(output, `${JSON.stringify(report, null, 2)}\n`);
  const message = `${suite} Go coverage: ${report.percent}% (${report.covered}/${report.total} statements)`;
  process.stdout.write(`${message}\n`);
  const summary = process.env.GITHUB_STEP_SUMMARY;
  if (summary !== undefined) {
    writeFileSync(
      summary,
      `## ${message}\n\nThis profile has its own denominator and is reported separately from browser coverage.\n\n`,
      { flag: "a" },
    );
  }
}

if (
  process.argv[1] !== undefined &&
  import.meta.url === pathToFileURL(process.argv[1]).href
) {
  main();
}

import { appendFileSync, existsSync, readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';

export function flakyTests(report) {
  const found = [];
  const walk = (suite, trail) => {
    for (const spec of suite.specs) {
      for (const test of spec.tests) {
        if (test.status !== 'flaky') {
          continue;
        }
        found.push({
          title: [...trail, spec.title].join(' › '),
          file: spec.file,
          line: spec.line,
          project: test.projectName,
          attempts: test.results.length,
        });
      }
    }
    for (const child of suite.suites ?? []) {
      walk(child, [...trail, child.title]);
    }
  };
  for (const file of report.suites) {
    walk(file, []);
  }
  return found;
}

export function summarize(report) {
  const flaky = flakyTests(report);
  if (flaky.length !== report.stats.flaky) {
    throw new Error(
      `the report counts ${String(report.stats.flaky)} flaky tests but ${String(flaky.length)} ` +
        'were found under its suites; the report shape has changed',
    );
  }
  return {
    expected: report.stats.expected,
    unexpected: report.stats.unexpected,
    skipped: report.stats.skipped,
    flaky,
  };
}

export function render(summary) {
  const lines = [
    `## Flaky tests: ${String(summary.flaky.length)}`,
    '',
    `${String(summary.expected)} passed, ${String(summary.unexpected)} failed, ` +
      `${String(summary.skipped)} skipped, ${String(summary.flaky.length)} passed only on retry.`,
  ];
  if (summary.flaky.length > 0) {
    lines.push('', '| test | file | project | attempts |', '|---|---|---|---|');
    for (const test of summary.flaky) {
      lines.push(
        `| ${test.title} | ${test.file}:${String(test.line)} | ${test.project} | ${String(test.attempts)} |`,
      );
    }
  }
  return `${lines.join('\n')}\n`;
}

function main() {
  const path = process.argv[2] ?? 'test-results/report.json';
  if (!existsSync(path)) {
    process.stderr.write(`flaky: no report at ${path}; Playwright did not finish\n`);
    process.exitCode = 1;
    return;
  }
  const report = JSON.parse(readFileSync(path, 'utf8'));
  const text = render(summarize(report));
  process.stdout.write(text);
  const summary = process.env.GITHUB_STEP_SUMMARY;
  if (summary !== undefined && summary !== '') {
    appendFileSync(summary, text);
  }
}

if (process.argv[1] !== undefined && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main();
}

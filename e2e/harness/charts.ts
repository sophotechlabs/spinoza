import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { CHART_DIR, CHART_PORT, CHART_REPO, E2E_DIR, TMP_DIR } from './paths';
import { background, mustRun, waitFor } from './run';
import { freePort } from './spinoza';

export const REPO_NAME = 'e2e-charts';

export const NEXT_VERSION = '0.2.0';

const CHART = join(E2E_DIR, 'fixtures', 'chart');

const SERVER = [
  "const http = require('node:http');",
  "const fs = require('node:fs');",
  "const path = require('node:path');",
  'const dir = process.env.CHART_DIR;',
  'const port = Number(process.env.CHART_PORT);',
  'http',
  '  .createServer((req, res) => {',
  "    const wanted = decodeURIComponent((req.url ?? '/').split('?')[0]);",
  '    fs.readFile(path.join(dir, wanted), (err, body) => {',
  '      if (err) {',
  '        res.statusCode = 404;',
  "        res.end('not here');",
  '        return;',
  '      }',
  '      res.end(body);',
  '    });',
  '  })',
  "  .listen(port, '127.0.0.1');",
].join('\n');

export function packageCharts(): void {
  mkdirSync(CHART_DIR, { recursive: true });
  mustRun('helm', [
    'package',
    CHART,
    '--version',
    NEXT_VERSION,
    '--app-version',
    '2.0.0',
    '--destination',
    CHART_DIR,
  ]);
  mustRun('helm', ['repo', 'index', CHART_DIR, '--url', CHART_REPO]);
}

export function writeRepositoryConfig(): void {
  const dir = join(TMP_DIR, 'home', '.config', 'helm');
  mkdirSync(dir, { recursive: true });
  writeFileSync(
    join(dir, 'repositories.yaml'),
    [
      'apiVersion: ""',
      'generated: "2026-01-01T00:00:00Z"',
      'repositories:',
      `  - name: ${REPO_NAME}`,
      `    url: ${CHART_REPO}`,
      '',
    ].join('\n'),
    { mode: 0o600 },
  );
}

export async function serveCharts(): Promise<number> {
  freePort(String(CHART_PORT));
  const pid = background('node', ['-e', SERVER], {
    env: { CHART_DIR, CHART_PORT: String(CHART_PORT) },
  });
  await waitFor('the chart repo to answer', 60, 250, async () => {
    try {
      const response = await fetch(`${CHART_REPO}/index.yaml`);
      return response.ok;
    } catch {
      return false;
    }
  });
  return pid;
}

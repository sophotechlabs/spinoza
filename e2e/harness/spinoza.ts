import { existsSync, readFileSync, rmSync } from 'node:fs';
import { ADDR, BASE_URL, BINARY, KUBECONFIG, REPO_DIR, TOKEN_FILE } from './paths';
import { background, mustRun, run, waitFor } from './run';

export function build(): void {
  if (process.env.SPINOZA_E2E_SKIP_BUILD === '1' && existsSync(BINARY)) {
    return;
  }
  mustRun('just', ['build'], { cwd: REPO_DIR });
}

export function stopStale(): void {
  const holders = run('lsof', ['-ti', `tcp:${ADDR.split(':')[1]}`, '-sTCP:LISTEN']).stdout.trim();
  if (holders === '') {
    return;
  }
  for (const pid of holders.split('\n')) {
    run('kill', [pid]);
  }
}

export async function start(extra: string[]): Promise<number> {
  rmSync(TOKEN_FILE, { force: true });
  const pid = background(BINARY, [
    '--addr',
    ADDR,
    '--kubeconfig',
    KUBECONFIG,
    '--token-file',
    TOKEN_FILE,
    '--log-level',
    'warn',
    ...extra,
  ]);
  await waitFor('spinoza to write its token', 120, 500, () => existsSync(TOKEN_FILE));
  await waitFor('spinoza to answer', 120, 500, async () => {
    try {
      const response = await fetch(`${BASE_URL}/`, { redirect: 'manual' });
      return response.status > 0;
    } catch {
      return false;
    }
  });
  return pid;
}

export function token(): string {
  return readFileSync(TOKEN_FILE, 'utf8').trim();
}

export function stop(pid: number): void {
  run('kill', [String(pid)]);
}

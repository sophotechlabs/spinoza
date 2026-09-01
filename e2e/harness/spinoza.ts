import { existsSync, mkdirSync, readFileSync, rmSync } from 'node:fs';
import { join } from 'node:path';
import { ADDR, BINARY, KUBECONFIG, REPO_DIR, TMP_DIR, TOKEN_FILE } from './paths';
import { background, mustRun, run, waitFor } from './run';

export function build(): void {
  if (process.env.SPINOZA_E2E_SKIP_BUILD === '1' && existsSync(BINARY)) {
    return;
  }
  mustRun('just', ['build'], { cwd: REPO_DIR });
}

export function stopStale(): void {
  freePort(ADDR.split(':')[1]);
}

export function freePort(port: string): void {
  const found = run('lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN']);
  const holders = found.stdout.trim();
  if (holders === '') {
    return;
  }
  for (const pid of holders.split('\n')) {
    const command = run('ps', ['-o', 'command=', '-p', pid]).stdout.trim();
    if (!command.startsWith(BINARY)) {
      throw new Error(
        `port ${port} is held by pid ${pid} running ${command}, which is not this checkout's ` +
          `spinoza at ${BINARY}. Two sessions have been given the same port; set SPINOZA_E2E_ADDR ` +
          `for this one rather than killing whatever is there.`,
      );
    }
    run('kill', [pid]);
  }
}

export interface Instance {
  pid: number;
  addr: string;
  baseURL: string;
  token: string;
}

export async function start(extra: string[]): Promise<number> {
  const started = await launch({
    addr: ADDR,
    kubeconfig: KUBECONFIG,
    tokenFile: TOKEN_FILE,
    home: join(TMP_DIR, 'home'),
    extra,
  });
  return started.pid;
}

interface Launch {
  addr: string;
  kubeconfig: string;
  tokenFile: string;
  home: string;
  extra: string[];
}

export async function launch(options: Launch): Promise<Instance> {
  rmSync(options.tokenFile, { force: true });
  mkdirSync(options.home, { recursive: true });
  const pid = background(
    BINARY,
    [
      '--addr',
      options.addr,
      '--kubeconfig',
      options.kubeconfig,
      '--token-file',
      options.tokenFile,
      '--log-level',
      process.env.SPINOZA_E2E_LOG ?? 'warn',
      ...options.extra,
    ],
    {
      env: {
        HELM_REPOSITORY_CACHE: join(options.home, '.cache', 'helm', 'repository'),
        HELM_REPOSITORY_CONFIG: join(options.home, '.config', 'helm', 'repositories.yaml'),
        HOME: options.home,
        XDG_CONFIG_HOME: join(options.home, '.config'),
      },
    },
  );
  const baseURL = `http://${options.addr}`;
  await waitFor('spinoza to write its token', 120, 500, () => existsSync(options.tokenFile));
  await waitFor('spinoza to answer', 120, 500, async () => {
    try {
      const response = await fetch(`${baseURL}/`, { redirect: 'manual' });
      return response.status > 0;
    } catch {
      return false;
    }
  });
  return {
    pid,
    addr: options.addr,
    baseURL,
    token: readFileSync(options.tokenFile, 'utf8').trim(),
  };
}

export function token(): string {
  return readFileSync(TOKEN_FILE, 'utf8').trim();
}

export function stop(pid: number): void {
  run('kill', [String(pid)]);
}

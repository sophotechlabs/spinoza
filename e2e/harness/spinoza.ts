import { existsSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { ADDR, BINARY, COVER_DIR, KUBECONFIG, REPO_DIR, TMP_DIR, TOKEN_FILE } from './paths';
import { background, mustRun, run, waitFor } from './run';
import { arrangeWorkspace } from './workspace';

const EXIT_ATTEMPTS = 60;
const EXIT_GAP = 500;

export function build(): void {
  if (process.env.SPINOZA_E2E_SKIP_BUILD === '1' && existsSync(BINARY)) {
    return;
  }
  mustRun('just', ['build-e2e'], { cwd: REPO_DIR });
}

export function resetCoverage(): void {
  rmSync(COVER_DIR, { recursive: true, force: true });
  mkdirSync(COVER_DIR, { recursive: true });
}

function alive(pid: number): boolean {
  const state = run('ps', ['-o', 'stat=', '-p', String(pid)]).stdout.trim();
  if (state === '') {
    return false;
  }
  return !state.startsWith('Z');
}

export async function freePort(port: string): Promise<void> {
  const found = run('lsof', ['-ti', `tcp:${port}`, '-sTCP:LISTEN']);
  const holders = found.stdout.trim();
  if (holders === '') {
    return;
  }
  const pids: number[] = [];
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
    pids.push(Number(pid));
  }
  await waitFor(`the stale spinoza on port ${port} to exit`, EXIT_ATTEMPTS, EXIT_GAP, () =>
    pids.every((pid) => !alive(pid)),
  );
}

export interface Instance {
  pid: number;
  addr: string;
  baseURL: string;
  token: string;
}

export async function start(extra: string[]): Promise<number> {
  const started = await launch({
    name: 'main',
    addr: ADDR,
    kubeconfig: KUBECONFIG,
    tokenFile: TOKEN_FILE,
    home: join(TMP_DIR, 'home'),
    extra,
  });
  return started.pid;
}

interface Launch {
  name: string;
  addr: string;
  kubeconfig: string;
  tokenFile: string;
  home: string;
  extra: string[];
}

export async function launch(options: Launch): Promise<Instance> {
  rmSync(options.tokenFile, { force: true });
  mkdirSync(options.home, { recursive: true });
  const counters = join(COVER_DIR, options.name);
  mkdirSync(counters, { recursive: true });
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
        GOCOVERDIR: counters,
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
  const value = readFileSync(options.tokenFile, 'utf8').trim();
  await arrangeWorkspace(baseURL, value);
  return {
    pid,
    addr: options.addr,
    baseURL,
    token: value,
  };
}

export function token(): string {
  return readFileSync(TOKEN_FILE, 'utf8').trim();
}

export async function stop(name: string, pid: number): Promise<void> {
  if (!alive(pid)) {
    return;
  }
  run('kill', [String(pid)]);
  try {
    await waitFor(`${name} to exit`, EXIT_ATTEMPTS, EXIT_GAP, () => !alive(pid));
  } catch {
    run('kill', ['-9', String(pid)]);
    writeFileSync(
      join(COVER_DIR, `${name}.unclean`),
      `pid ${String(pid)} was still running ${String((EXIT_ATTEMPTS * EXIT_GAP) / 1000)}s ` +
        'after SIGTERM and was killed, so its coverage counters were never written\n',
    );
  }
}

import { spawn, spawnSync } from 'node:child_process';

export interface RunResult {
  code: number;
  stdout: string;
  stderr: string;
}

export interface RunOptions {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
}

export function run(command: string, args: string[], options: RunOptions = {}): RunResult {
  const done = spawnSync(command, args, {
    encoding: 'utf8',
    cwd: options.cwd,
    env: { ...process.env, ...options.env },
  });
  if (done.error) {
    throw done.error;
  }
  return {
    code: done.status ?? 1,
    stdout: done.stdout ?? '',
    stderr: done.stderr ?? '',
  };
}

export function mustRun(command: string, args: string[], options: RunOptions = {}): string {
  const result = run(command, args, options);
  if (result.code !== 0) {
    throw new Error(
      `${command} ${args.join(' ')} exited ${String(result.code)}\n${result.stderr || result.stdout}`,
    );
  }
  return result.stdout;
}

export function background(command: string, args: string[], options: RunOptions = {}): number {
  const child = spawn(command, args, {
    detached: true,
    stdio: 'ignore',
    cwd: options.cwd,
    env: { ...process.env, ...options.env },
  });
  child.unref();
  if (child.pid === undefined) {
    throw new Error(`${command} did not start`);
  }
  return child.pid;
}

export async function waitFor(
  what: string,
  attempts: number,
  gap: number,
  check: () => boolean | Promise<boolean>,
): Promise<void> {
  for (let index = 0; index < attempts; index += 1) {
    if (await check()) {
      return;
    }
    await new Promise((wake) => setTimeout(wake, gap));
  }
  throw new Error(`timed out waiting for ${what}`);
}

import { spawn, spawnSync } from 'node:child_process';
import { closeSync, mkdirSync, openSync } from 'node:fs';
import { dirname } from 'node:path';

export interface RunResult {
  code: number;
  stdout: string;
  stderr: string;
}

export interface RunOptions {
  cwd?: string;
  env?: NodeJS.ProcessEnv;
  input?: string;
}

export function run(command: string, args: string[], options: RunOptions = {}): RunResult {
  const done = spawnSync(command, args, {
    encoding: 'utf8',
    cwd: options.cwd,
    env: { ...process.env, ...options.env },
    input: options.input,
    maxBuffer: 64 * 1024 * 1024,
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

export function printable(text: string): string {
  return text.replace(/[\u{10000}-\u{10FFFF}\uFE0F\p{Cs}]/gu, '');
}

export function mustRun(command: string, args: string[], options: RunOptions = {}): string {
  const result = run(command, args, options);
  if (result.code !== 0) {
    throw new Error(
      printable(
        `${command} ${args.join(' ')} exited ${String(result.code)}\n${result.stderr || result.stdout}`,
      ),
    );
  }
  return result.stdout;
}

export interface BackgroundOptions extends RunOptions {
  log: string;
}

export function background(command: string, args: string[], options: BackgroundOptions): number {
  mkdirSync(dirname(options.log), { recursive: true });
  const sink = openSync(options.log, 'a');
  const child = spawn(command, args, {
    detached: true,
    stdio: ['ignore', sink, sink],
    cwd: options.cwd,
    env: { ...process.env, ...options.env },
  });
  closeSync(sink);
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

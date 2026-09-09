import { existsSync, readFileSync } from 'node:fs';
import { STATE_FILE } from './paths';
import { teardown } from './fixtures';
import { deleteCluster } from './cluster';
import { stop } from './spinoza';

interface Stopping {
  pid: number;
  charts?: number;
  sides?: Record<string, { pid: number }>;
}

export default async function globalTeardown(): Promise<void> {
  if (existsSync(STATE_FILE)) {
    const state = JSON.parse(readFileSync(STATE_FILE, 'utf8')) as Stopping;
    await stop('main', state.pid);
    for (const [name, one] of Object.entries(state.sides ?? {})) {
      await stop(name, one.pid);
    }
    if (state.charts !== undefined) {
      await stop('charts', state.charts);
    }
  }
  if (process.env.SPINOZA_E2E_KEEP === '1') {
    return;
  }
  teardown();
  if (process.env.SPINOZA_E2E_DELETE_CLUSTER === '1') {
    deleteCluster();
  }
}

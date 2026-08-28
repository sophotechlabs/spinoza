import { existsSync, readFileSync } from 'node:fs';
import { STATE_FILE } from './paths';
import { teardown } from './fixtures';
import { deleteCluster } from './cluster';
import { stop } from './spinoza';

export default function globalTeardown(): void {
  if (existsSync(STATE_FILE)) {
    const state = JSON.parse(readFileSync(STATE_FILE, 'utf8')) as { pid: number };
    stop(state.pid);
  }
  if (process.env.SPINOZA_E2E_KEEP === '1') {
    return;
  }
  teardown();
  if (process.env.SPINOZA_E2E_DELETE_CLUSTER === '1') {
    deleteCluster();
  }
}

import { mkdirSync, writeFileSync } from 'node:fs';
import { BASE_URL, STATE_FILE, TMP_DIR } from './paths';
import { ensureCluster, exportKubeconfig, refuseAnythingButKind } from './cluster';
import { seed, waitForFixtures } from './fixtures';
import { build, start, stopStale, token } from './spinoza';

export default async function globalSetup(): Promise<void> {
  mkdirSync(TMP_DIR, { recursive: true });
  ensureCluster();
  exportKubeconfig();
  refuseAnythingButKind();
  seed();
  waitForFixtures();
  build();
  stopStale();
  const pid = await start(['--node-shell']);
  writeFileSync(STATE_FILE, JSON.stringify({ pid, baseURL: BASE_URL, token: token() }, null, 2));
}

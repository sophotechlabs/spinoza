import { mkdirSync, writeFileSync } from 'node:fs';
import { BASE_URL, STATE_FILE, STORAGE_STATE, TMP_DIR } from './paths';
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
  const value = token();
  writeFileSync(STATE_FILE, JSON.stringify({ pid, baseURL: BASE_URL, token: value }, null, 2));
  writeFileSync(
    STORAGE_STATE,
    JSON.stringify({
      cookies: [
        {
          name: 'spinoza_token',
          value,
          domain: '127.0.0.1',
          path: '/',
          expires: -1,
          httpOnly: true,
          secure: false,
          sameSite: 'Strict',
        },
      ],
      origins: [],
    }),
  );
}

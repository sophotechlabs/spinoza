import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { BASE_URL, NOWHERE_KUBECONFIG, STATE_FILE, STORAGE_STATE, TMP_DIR, sideAddr } from './paths';
import { KUBECONFIG } from './paths';
import { ensureCluster, exportKubeconfig, readonlyKubeconfig, refuseAnythingButKind } from './cluster';
import { seed, seedGitops, seedHelm, seedScale, waitForFixtures } from './fixtures';
import { packageCharts, serveCharts, writeRepositoryCache, writeRepositoryConfig } from './charts';
import { build, freePort, launch, start, stopStale, token } from './spinoza';
import type { Instance } from './spinoza';

function unreachableKubeconfig(): string {
  const path = NOWHERE_KUBECONFIG;
  writeFileSync(
    path,
    [
      'apiVersion: v1',
      'kind: Config',
      'current-context: nowhere',
      'clusters:',
      '  - name: nowhere',
      '    cluster:',
      '      server: https://127.0.0.1:1',
      'users:',
      '  - name: nobody',
      '    user: {}',
      'contexts:',
      '  - name: nowhere',
      '    context:',
      '      cluster: nowhere',
      '      user: nobody',
      '',
    ].join('\n'),
    { mode: 0o600 },
  );
  return path;
}

async function side(name: string, index: number, kubeconfig: string, extra: string[]): Promise<Instance> {
  const addr = sideAddr(index);
  freePort(addr.split(':')[1]);
  return launch({
    addr,
    kubeconfig,
    tokenFile: join(TMP_DIR, `token-${name}`),
    home: join(TMP_DIR, `home-${name}`),
    extra,
  });
}

export default async function globalSetup(): Promise<void> {
  mkdirSync(TMP_DIR, { recursive: true });
  for (const name of [
    'home',
    'home-readonly',
    'home-toolless',
    'home-nowhere',
    'home-traffic',
    'home-profiled',
  ]) {
    rmSync(join(TMP_DIR, name), { recursive: true, force: true });
  }
  ensureCluster();
  exportKubeconfig();
  refuseAnythingButKind();
  seed();
  if (process.env.SPINOZA_E2E_TIER === 'full') {
    seedGitops();
    seedScale();
  }
  seedHelm();
  waitForFixtures();
  build();
  packageCharts();
  writeRepositoryConfig();
  writeRepositoryCache();
  const charts = await serveCharts();
  stopStale();
  const pid = await start(['--node-shell']);
  const value = token();
  const readonly = await side('readonly', 1, readonlyKubeconfig(), []);
  const toolless = await side('toolless', 2, readonlyKubeconfig(), [
    '--helm',
    '/nonexistent/helm',
    '--kubectl',
    '/nonexistent/kubectl',
  ]);
  const nowhere = await side('nowhere', 3, unreachableKubeconfig(), []);
  const traffic = await side('traffic', 4, KUBECONFIG, ['--prometheus', 'e2e/fake-prom:9090']);
  const profiled = await side('profiled', 5, KUBECONFIG, ['--pprof']);
  writeFileSync(
    STATE_FILE,
    JSON.stringify(
      {
        pid,
        baseURL: BASE_URL,
        token: value,
        charts,
        sides: { readonly, toolless, nowhere, traffic, profiled },
      },
      null,
      2,
    ),
  );
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

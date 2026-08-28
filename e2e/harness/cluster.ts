import { mkdirSync, writeFileSync } from 'node:fs';
import { CLUSTER, CONTEXT, KUBECONFIG, TMP_DIR } from './paths';
import { mustRun, run } from './run';

const LOOPBACK = ['127.0.0.1', 'localhost', '0.0.0.0'];

export function ensureCluster(): void {
  const listed = mustRun('kind', ['get', 'clusters']);
  if (listed.split('\n').includes(CLUSTER)) {
    return;
  }
  mustRun('kind', ['create', 'cluster', '--name', CLUSTER, '--wait', '120s']);
}

export function exportKubeconfig(): void {
  mkdirSync(TMP_DIR, { recursive: true });
  const config = mustRun('kind', ['get', 'kubeconfig', '--name', CLUSTER]);
  writeFileSync(KUBECONFIG, config, { mode: 0o600 });
}

function serverOf(): string {
  return mustRun('kubectl', [
    '--kubeconfig',
    KUBECONFIG,
    'config',
    'view',
    '--minify',
    '-o',
    'jsonpath={.clusters[0].cluster.server}',
  ]).trim();
}

function contextsIn(): string[] {
  return mustRun('kubectl', ['--kubeconfig', KUBECONFIG, 'config', 'get-contexts', '-o', 'name'])
    .split('\n')
    .map((name) => name.trim())
    .filter((name) => name !== '');
}

export function refuseAnythingButKind(): void {
  const contexts = contextsIn();
  if (contexts.length !== 1) {
    throw new Error(`the e2e kubeconfig must hold exactly one context, found ${contexts.join(', ')}`);
  }
  if (contexts[0] !== CONTEXT) {
    throw new Error(`the e2e kubeconfig must hold ${CONTEXT}, found ${contexts[0]}`);
  }
  const server = serverOf();
  const host = new URL(server).hostname;
  if (!LOOPBACK.includes(host)) {
    throw new Error(`refusing to run against ${server}, which is not a local kind cluster`);
  }
}

export function kubectl(args: string[]): string {
  return mustRun('kubectl', ['--kubeconfig', KUBECONFIG, '--context', CONTEXT, ...args]);
}

export function kubectlSoft(args: string[]): number {
  return run('kubectl', ['--kubeconfig', KUBECONFIG, '--context', CONTEXT, ...args]).code;
}

export function deleteCluster(): void {
  run('kind', ['delete', 'cluster', '--name', CLUSTER]);
}

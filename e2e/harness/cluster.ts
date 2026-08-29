import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { CLUSTER, CONTEXT, KUBECONFIG, NAMESPACE, REPO_DIR, TMP_DIR } from './paths';
import { mustRun, run } from './run';

const LOOPBACK = ['127.0.0.1', 'localhost', '0.0.0.0'];

export function ensureCluster(): void {
  let recipe = 'cluster-e2e';
  if (process.env.SPINOZA_E2E_TIER === 'full') {
    recipe = 'cluster-full';
  }
  mustRun('just', [recipe], {
    cwd: REPO_DIR,
    env: { SPINOZA_KIND_CLUSTER: CLUSTER },
  });
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

export function kubectlApply(document: string): string {
  return mustRun('kubectl', ['--kubeconfig', KUBECONFIG, '--context', CONTEXT, 'apply', '-f', '-'], {
    input: document,
  });
}

export function kubectlSoft(args: string[]): number {
  return run('kubectl', ['--kubeconfig', KUBECONFIG, '--context', CONTEXT, ...args]).code;
}

export function helm(args: string[]): string {
  return mustRun('helm', [
    '--kubeconfig',
    KUBECONFIG,
    '--kube-context',
    CONTEXT,
    ...args,
  ]);
}

export function helmSoft(args: string[]): number {
  return run('helm', ['--kubeconfig', KUBECONFIG, '--kube-context', CONTEXT, ...args]).code;
}

export function deleteCluster(): void {
  run('kind', ['delete', 'cluster', '--name', CLUSTER]);
}

function caOf(): string {
  return mustRun('kubectl', [
    '--kubeconfig',
    KUBECONFIG,
    'config',
    'view',
    '--raw',
    '--minify',
    '-o',
    'jsonpath={.clusters[0].cluster.certificate-authority-data}',
  ]).trim();
}

export function readonlyKubeconfig(): string {
  const path = join(TMP_DIR, 'kubeconfig-readonly');
  const token = kubectl([
    '--namespace',
    NAMESPACE,
    'create',
    'token',
    'readonly',
    '--duration=2h',
  ]).trim();
  const document = [
    'apiVersion: v1',
    'kind: Config',
    `current-context: ${CONTEXT}`,
    'clusters:',
    `  - name: ${CLUSTER}`,
    '    cluster:',
    `      server: ${serverOf()}`,
    `      certificate-authority-data: ${caOf()}`,
    'users:',
    '  - name: readonly',
    '    user:',
    `      token: ${token}`,
    'contexts:',
    `  - name: ${CONTEXT}`,
    '    context:',
    `      cluster: ${CLUSTER}`,
    '      user: readonly',
    `      namespace: ${NAMESPACE}`,
    '',
  ].join('\n');
  writeFileSync(path, document, { mode: 0o600 });
  return path;
}

export function nodeContainer(suffix: string): string {
  return `${CLUSTER}-${suffix}`;
}

export function pauseContainer(name: string): void {
  mustRun('docker', ['pause', name]);
}

export function unpauseContainer(name: string): void {
  run('docker', ['unpause', name]);
}

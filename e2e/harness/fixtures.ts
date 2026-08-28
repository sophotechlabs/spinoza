import { join } from 'node:path';
import { E2E_DIR, NAMESPACE } from './paths';
import { kubectl, kubectlSoft } from './cluster';

const MANIFEST = join(E2E_DIR, 'fixtures', 'workloads.yaml');

export function seed(): void {
  kubectl(['apply', '-f', MANIFEST]);
}

export function waitForFixtures(): void {
  kubectl([
    'wait',
    '--namespace',
    NAMESPACE,
    '--for=condition=Available',
    'deployment/healthy',
    'deployment/chatty',
    '--timeout=180s',
  ]);
  kubectl([
    'wait',
    '--namespace',
    NAMESPACE,
    '--for=condition=Ready',
    'pod/noshell',
    '--timeout=120s',
  ]);
}

export function teardown(): void {
  kubectlSoft(['delete', '-f', MANIFEST, '--ignore-not-found', '--wait=false']);
}

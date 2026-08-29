import { join } from 'node:path';
import { E2E_DIR, NAMESPACE } from './paths';
import { helm, helmSoft, kubectl, kubectlApply, kubectlSoft } from './cluster';

const FIXTURES = join(E2E_DIR, 'fixtures');

const WORKLOADS = join(FIXTURES, 'workloads.yaml');
const CRD = join(FIXTURES, 'crd.yaml');
const WIDGETS = join(FIXTURES, 'widgets.yaml');
const RBAC = join(FIXTURES, 'rbac.yaml');
const PROMETHEUS = join(FIXTURES, 'prometheus.yaml');
const CHART = join(FIXTURES, 'chart');
const GITOPS = join(FIXTURES, 'gitops.yaml');

export const RELEASE = 'e2e-release';

export const DOOMED = 'e2e-doomed';

export function seed(): void {
  kubectl(['apply', '-f', WORKLOADS]);
  kubectl(['apply', '-f', RBAC]);
  kubectl(['apply', '-f', PROMETHEUS]);
  kubectl(['apply', '-f', CRD]);
  kubectl([
    'wait',
    '--for=condition=Established',
    'crd/widgets.spinoza.test',
    '--timeout=120s',
  ]);
  kubectl(['apply', '-f', WIDGETS]);
}

export function seedHelm(): void {
  helm([
    'upgrade',
    '--install',
    RELEASE,
    CHART,
    '--namespace',
    NAMESPACE,
    '--wait',
    '--timeout',
    '3m',
  ]);
  helm([
    'upgrade',
    RELEASE,
    CHART,
    '--namespace',
    NAMESPACE,
    '--set',
    'greeting=hello from revision two',
    '--wait',
    '--timeout',
    '3m',
  ]);
  helm([
    'upgrade',
    '--install',
    DOOMED,
    CHART,
    '--namespace',
    NAMESPACE,
    '--set',
    'replicaCount=0',
    '--wait',
    '--timeout',
    '3m',
  ]);
}

export function seedGitops(): void {
  kubectl(['apply', '-f', GITOPS]);
}

export const SCALE_NAMESPACE = 'e2e-scale';

export const SCALE_CONFIGMAPS = 1500;

export const SCALE_WORKLOADS = 300;

function scaleDocuments(): string {
  const parts = [`apiVersion: v1\nkind: Namespace\nmetadata:\n  name: ${SCALE_NAMESPACE}`];
  for (let index = 0; index < SCALE_CONFIGMAPS; index += 1) {
    parts.push(
      [
        'apiVersion: v1',
        'kind: ConfigMap',
        'metadata:',
        `  name: bulk-${String(index).padStart(4, '0')}`,
        `  namespace: ${SCALE_NAMESPACE}`,
        'data:',
        `  index: "${String(index)}"`,
      ].join('\n'),
    );
  }
  for (let index = 0; index < SCALE_WORKLOADS; index += 1) {
    const name = `idle-${String(index).padStart(4, '0')}`;
    parts.push(
      [
        'apiVersion: apps/v1',
        'kind: Deployment',
        'metadata:',
        `  name: ${name}`,
        `  namespace: ${SCALE_NAMESPACE}`,
        'spec:',
        '  replicas: 0',
        '  selector:',
        '    matchLabels:',
        `      app: ${name}`,
        '  template:',
        '    metadata:',
        '      labels:',
        `        app: ${name}`,
        '    spec:',
        '      containers:',
        '        - name: idle',
        '          image: busybox:latest',
        "          command: ['sh', '-c', 'sleep 3600']",
      ].join('\n'),
    );
  }
  return parts.join('\n---\n');
}

export function seedScale(): void {
  kubectlApply(scaleDocuments());
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
    'pod/shellable',
    'pod/fake-prom',
    '--timeout=120s',
  ]);
}

export function teardown(): void {
  helmSoft(['uninstall', RELEASE, '--namespace', NAMESPACE, '--ignore-not-found']);
  helmSoft(['uninstall', DOOMED, '--namespace', NAMESPACE, '--ignore-not-found']);
  kubectlSoft(['delete', '-f', PROMETHEUS, '--ignore-not-found', '--wait=false']);
  kubectlSoft(['delete', '-f', GITOPS, '--ignore-not-found', '--wait=false']);
  kubectlSoft(['delete', 'namespace', SCALE_NAMESPACE, '--ignore-not-found', '--wait=false']);
  kubectlSoft(['delete', '-f', WIDGETS, '--ignore-not-found', '--wait=false']);
  kubectlSoft(['delete', '-f', CRD, '--ignore-not-found', '--wait=false']);
  kubectlSoft(['delete', '-f', RBAC, '--ignore-not-found', '--wait=false']);
  kubectlSoft(['delete', '-f', WORKLOADS, '--ignore-not-found', '--wait=false']);
}

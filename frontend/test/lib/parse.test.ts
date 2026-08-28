import { describe, expect, it } from 'vitest';
import {
  parseKindComparison,
  parseActionResult,
  parseCatalog,
  parseCounts,
  parseDebugSession,
  parseDebugSupport,
  parseEvents,
  parseExecSupport,
  parseFluxActionResult,
  parseFluxDashboard,
  parseForward,
  parseForwards,
  parseGraph,
  parseHelmActionResult,
  parseHelmChartVersions,
  parseHelmReleases,
  parseMetricHistory,
  parseUpdateResult,
  parseUpdateStatus,
  parseMetrics,
  parseObjectDetail,
  parseRow,
} from '../../src/lib/parse';

describe('parseRow', () => {
  it('fills in every field a truncated row left out', () => {
    expect(parseRow({ uid: 'u' })).toEqual({
      uid: 'u',
      name: '',
      namespace: '',
      createdAt: '',
      cells: [],
      containers: undefined,
    });
  });

  it('narrows an unknown container state to waiting', () => {
    const row = parseRow({
      uid: 'u',
      containers: [{ name: 'app', state: 'sleeping', ready: 'yes', restarts: '3' }],
    });

    expect(row.containers).toEqual([
      {
        name: 'app',
        state: 'waiting',
        reason: undefined,
        ready: false,
        restarts: 0,
        init: false,
        ephemeral: undefined,
      },
    ]);
  });

  it('drops cells that are not strings', () => {
    expect(parseRow({ cells: ['a', 3, null] }).cells).toEqual(['a', '', '']);
  });
});

describe('parseObjectDetail', () => {
  it('reads the decoded entries of a secret', () => {
    const detail = parseObjectDetail({
      apiVersion: 'v1',
      kind: 'Secret',
      name: 'creds',
      namespace: 'prod',
      uid: 'u',
      createdAt: 'now',
      data: [
        { key: 'password', value: 'hunter2', bytes: 7 },
        { key: 'keystore', value: '/v8A', bytes: 3, binary: true },
      ],
      yaml: 'kind: Secret\n',
    });

    expect(detail.data).toEqual([
      { key: 'password', value: 'hunter2', bytes: 7, binary: undefined },
      { key: 'keystore', value: '/v8A', bytes: 3, binary: true },
    ]);
  });

  it('leaves the entries out for anything that is not a secret', () => {
    const detail = parseObjectDetail({
      apiVersion: 'v1',
      kind: 'Pod',
      name: 'web',
      namespace: 'prod',
      uid: 'u',
      createdAt: 'now',
      yaml: 'kind: Pod\n',
    });

    expect(detail.data).toBeUndefined();
  });

  it('groups the per-kind fields into facets', () => {
    const detail = parseObjectDetail({
      apiVersion: 'v1',
      kind: 'Pod',
      name: 'web',
      namespace: 'prod',
      uid: 'u',
      createdAt: 'now',
      containers: ['app', 'sidecar'],
      replicas: 3,
      schedulable: false,
      suspended: true,
      handledAt: 'token',
      yaml: 'kind: Pod\n',
    });

    expect(detail.pod).toEqual({ containers: ['app', 'sidecar'] });
    expect(detail.workload).toEqual({ replicas: 3 });
    expect(detail.node).toEqual({ schedulable: false });
    expect(detail.suspended).toBe(true);
    expect(detail.flux).toEqual({ handledAt: 'token' });
  });

  it('takes a suspended flag with no flux fields beside it', () => {
    const detail = parseObjectDetail({
      apiVersion: 'batch/v1',
      kind: 'CronJob',
      name: 'nightly',
      suspended: false,
      yaml: '',
    });

    expect(detail.suspended).toBe(false);
    expect(detail.flux).toBeUndefined();
  });

  it('leaves every facet out when the object carries none of them', () => {
    const detail = parseObjectDetail({ kind: 'ConfigMap', yaml: '' });

    expect(detail.pod).toBeUndefined();
    expect(detail.workload).toBeUndefined();
    expect(detail.node).toBeUndefined();
    expect(detail.suspended).toBeUndefined();
    expect(detail.flux).toBeUndefined();
  });

  it('keeps labels and annotations that are string maps and drops the rest', () => {
    const detail = parseObjectDetail({ labels: { a: 'x', b: 2 }, annotations: 'nope' });

    expect(detail.labels).toEqual({ a: 'x' });
    expect(detail.annotations).toBeUndefined();
  });

  it('parses owners, conditions and ports', () => {
    const detail = parseObjectDetail({
      owners: [{ kind: 'ReplicaSet', name: 'web-1', uid: 'o' }],
      conditions: [{ type: 'Ready', status: 'True', reason: 'ok' }],
      ports: [{ name: 'http', port: 8080, protocol: 'TCP' }],
    });

    expect(detail.owners).toEqual([{ kind: 'ReplicaSet', name: 'web-1', uid: 'o' }]);
    expect(detail.conditions?.[0].type).toBe('Ready');
    expect(detail.ports).toEqual([{ name: 'http', port: 8080, protocol: 'TCP' }]);
  });

  it('survives a payload that is not an object at all', () => {
    expect(parseObjectDetail('nope').kind).toBe('');
  });
});

describe('parseEvents', () => {
  it('narrows an unknown event type to Normal', () => {
    const events = parseEvents([{ type: 'Catastrophe', reason: 'Boom', count: 2 }]);

    expect(events[0].type).toBe('Normal');
    expect(events[0].count).toBe(2);
  });

  it('keeps a Warning', () => {
    expect(parseEvents([{ type: 'Warning' }])[0].type).toBe('Warning');
  });
});

describe('parseForwards', () => {
  it('treats an unknown forward state as failed rather than running', () => {
    expect(parseForward({ id: 'pf-1', state: 'zombie' }).state).toBe('failed');
    expect(parseForward({ id: 'pf-1', state: 'running' }).state).toBe('running');
  });

  it('parses a list', () => {
    expect(parseForwards([{ id: 'a' }, { id: 'b' }]).map((one) => one.id)).toEqual(['a', 'b']);
  });
});

describe('parseFluxDashboard', () => {
  it('narrows an unrecognised ready value to Unknown', () => {
    const dash = parseFluxDashboard({
      groups: [{ name: 'Sources', resources: [{ name: 'repo', ready: 'Maybe' }] }],
    });

    expect(dash.groups[0].resources[0].ready).toBe('Unknown');
  });

  it('keeps a blank ready value, which means no status yet', () => {
    const dash = parseFluxDashboard({ groups: [{ resources: [{ ready: '' }] }] });

    expect(dash.groups[0].resources[0].ready).toBe('');
  });

  it('returns an empty dashboard for a payload with no groups', () => {
    expect(parseFluxDashboard({}).groups).toEqual([]);
  });
});

describe('parseGraph', () => {
  it('narrows unknown node categories and edge kinds', () => {
    const graph = parseGraph({
      nodes: [{ id: 'a', category: 'wat', ready: 'True' }],
      edges: [{ from: 'a', to: 'b', kind: 'wat' }],
    });

    expect(graph.nodes[0].category).toBe('managed');
    expect(graph.edges[0].kind).toBe('manages');
  });
});

describe('parseMetrics', () => {
  it('zeroes usage numbers the backend sent as strings', () => {
    const metrics = parseMetrics({ pods: { 'prod/web': { cpuMilli: '150', memoryMi: 192 } } });

    expect(metrics.pods['prod/web']).toEqual({
      cpuMilli: 0,
      memoryMi: 192,
      cpuPercent: 0,
      memPercent: 0,
      cpuAllocatableMilli: 0,
      memAllocatableMi: 0,
    });
  });

  it('gives empty maps for a payload with neither pods nor nodes', () => {
    expect(parseMetrics({})).toEqual({ pods: {}, nodes: {}, error: undefined });
  });
});

describe('parseMetricHistory', () => {
  it('keeps the two series apart and drops junk points', () => {
    const history = parseMetricHistory({
      namespace: 'prod',
      pod: 'web',
      cpu: [{ at: 1, value: 0.5 }],
      memory: [{ at: 2, value: 'x' }],
    });

    expect(history.cpu).toEqual([{ at: 1, value: 0.5 }]);
    expect(history.memory).toEqual([{ at: 2, value: 0 }]);
  });

  it('carries whether spinoza measured it and how far back that reaches', () => {
    const history = parseMetricHistory({
      namespace: 'prod',
      pod: 'web',
      sampled: true,
      since: 1785434552000,
      cpu: [],
      memory: [],
    });

    expect(history.sampled).toBe(true);
    expect(history.since).toBe(1785434552000);
  });

  it('leaves both out when a metrics database answered', () => {
    const history = parseMetricHistory({ namespace: 'prod', pod: 'web', cpu: [], memory: [] });

    expect(history.sampled).toBeUndefined();
    expect(history.since).toBeUndefined();
  });
});

describe('parseCatalog', () => {
  it('parses categories and their descriptors', () => {
    const catalog = parseCatalog({
      categories: [{ name: 'Workloads', resources: [{ kind: 'Pod', namespaced: true }] }],
      error: 'partial',
    });

    expect(catalog.categories[0].resources[0].kind).toBe('Pod');
    expect(catalog.categories[0].resources[0].namespaced).toBe(true);
    expect(catalog.error).toBe('partial');
  });
});

describe('parseCounts', () => {
  it('keeps numeric counts only', () => {
    expect(parseCounts({ counts: { '/v1/pods': 3, '/v1/nodes': 'x' } }).counts).toEqual({
      '/v1/pods': 3,
    });
  });

  it('is empty when the payload has no counts', () => {
    expect(parseCounts({})).toEqual({ counts: {}, errors: undefined });
  });

  it('keeps the per-resource errors the server reports', () => {
    expect(parseCounts({ errors: { '/v1/secrets': 'forbidden' } }).errors).toEqual({
      '/v1/secrets': 'forbidden',
    });
  });
});

describe('the remaining action payloads', () => {
  it('parses an action result with its pod outcomes', () => {
    const result = parseActionResult({
      action: 'drain',
      message: 'done',
      dryRun: true,
      pods: [{ namespace: 'prod', name: 'web', outcome: 'evicted' }],
    });

    expect(result.pods).toEqual([
      { namespace: 'prod', name: 'web', outcome: 'evicted', reason: undefined },
    ]);
    expect(result.dryRun).toBe(true);
  });

  it('parses a flux action result', () => {
    expect(parseFluxActionResult({ action: 'reconcile', requestedAt: 'token' })).toEqual({
      action: 'reconcile',
      requestedAt: 'token',
    });
  });

  it('narrows an unknown shell state to unknown', () => {
    expect(parseExecSupport({ shell: 'maybe' }).shell).toBe('unknown');
    expect(parseExecSupport({ shell: 'present' }).shell).toBe('present');
  });

  it('parses a debug session and its support probe', () => {
    expect(parseDebugSession({ container: 'dbg', created: true, image: 'busybox' }).created).toBe(
      true,
    );
    expect(parseDebugSupport({ allowed: true, image: 'busybox' }).allowed).toBe(true);
  });
});

describe('the helm upgrade payloads', () => {
  it('parses a release with its flux owner', () => {
    const got = parseHelmReleases({
      releases: [
        {
          name: 'podinfo',
          namespace: 'demo',
          chart: 'podinfo',
          chartVersion: '6.9.2',
          appVersion: '6.9.2',
          revision: 1,
          status: 'deployed',
          updated: '2026-08-11T09:30:00Z',
          fluxRef: {
            group: 'helm.toolkit.fluxcd.io',
            version: 'v2',
            resource: 'helmreleases',
            namespace: 'demo',
            name: 'podinfo',
          },
        },
      ],
    });

    expect(got.releases[0].fluxRef).toEqual({
      group: 'helm.toolkit.fluxcd.io',
      version: 'v2',
      resource: 'helmreleases',
      namespace: 'demo',
      name: 'podinfo',
    });
  });

  it('leaves the owner absent for a hand-installed release', () => {
    const got = parseHelmReleases({
      releases: [{ name: 'podinfo', namespace: 'demo', fluxRef: null }],
    });

    expect(got.releases[0].fluxRef).toBeUndefined();
  });

  it('parses a dry-run result with its manifest', () => {
    const got = parseHelmActionResult({
      action: 'upgrade',
      message: 'server render of podinfo 6.15.1',
      dryRun: true,
      manifest: 'kind: ConfigMap\n',
    });

    expect(got.dryRun).toBe(true);
    expect(got.manifest).toBe('kind: ConfigMap\n');
  });

  it('parses the grouped chart versions', () => {
    const got = parseHelmChartVersions({
      chart: 'podinfo',
      repos: [
        { name: 'podinfo', url: 'https://example.com', versions: ['6.15.1', '6.14.0'] },
        { url: 'oci://ghcr.io/acme/charts', oci: true, versions: ['1.0.0'] },
      ],
      error: 'one repo failed',
    });

    expect(got.chart).toBe('podinfo');
    expect(got.repos[0].name).toBe('podinfo');
    expect(got.repos[0].versions).toEqual(['6.15.1', '6.14.0']);
    expect(got.repos[1].oci).toBe(true);
    expect(got.error).toBe('one repo failed');
  });

  it('reads an empty versions payload', () => {
    expect(parseHelmChartVersions({})).toEqual({ chart: '', repos: [], error: undefined });
  });
});

describe('an event object detail', () => {
  it('carries the event facts the overview shows', () => {
    const detail = parseObjectDetail({
      apiVersion: 'v1',
      kind: 'Event',
      name: 'web.17abc',
      namespace: 'prod',
      uid: 'u-1',
      createdAt: '2026-08-18T09:00:00Z',
      yaml: 'kind: Event\n',
      event: {
        type: 'Warning',
        reason: 'BackOff',
        message: 'Back-off restarting failed container',
        object: 'Pod prod/web-0',
        source: 'kubelet on node-1',
        count: 42,
        firstSeen: '2026-08-18T09:00:00Z',
        lastSeen: '2026-08-18T10:00:00Z',
      },
    });

    expect(detail.event).toEqual({
      type: 'Warning',
      reason: 'BackOff',
      message: 'Back-off restarting failed container',
      object: 'Pod prod/web-0',
      source: 'kubelet on node-1',
      count: 42,
      firstSeen: '2026-08-18T09:00:00Z',
      lastSeen: '2026-08-18T10:00:00Z',
    });
  });

  it('leaves the facts out for anything that is not an event', () => {
    const detail = parseObjectDetail({
      apiVersion: 'v1',
      kind: 'Pod',
      name: 'web-0',
      yaml: 'kind: Pod\n',
    });

    expect(detail.event).toBeUndefined();
  });
});

describe('parseKindComparison', () => {
  it('reads the verdicts and the tally', () => {
    const got = parseKindComparison({
      resource: 'deployments',
      leftContext: 'p-mk1',
      rightContext: 'p-mk2',
      namespace: 'flux-system',
      objects: [
        { namespace: 'flux-system', name: 'web', verdict: 'differs', lines: 4 },
        { name: 'cluster-admin', verdict: 'onlyThere' },
      ],
      same: 1,
      differs: 2,
      onlyHere: 3,
      onlyThere: 4,
      matchedByName: true,
    });

    expect(got.objects[0]).toEqual({
      namespace: 'flux-system',
      name: 'web',
      verdict: 'differs',
      lines: 4,
    });
    expect(got.objects[1].namespace).toBeUndefined();
    expect([got.same, got.differs, got.onlyHere, got.onlyThere]).toEqual([1, 2, 3, 4]);
    expect(got.matchedByName).toBe(true);
  });

  it('treats a verdict it does not know as same rather than inventing drift', () => {
    const got = parseKindComparison({
      resource: 'deployments',
      leftContext: 'a',
      rightContext: 'b',
      objects: [{ name: 'web', verdict: 'sideways' }],
      same: 0,
      differs: 0,
      onlyHere: 0,
      onlyThere: 0,
    });

    expect(got.objects[0].verdict).toBe('same');
  });

  it('fills in what the backend left out', () => {
    const got = parseKindComparison({});

    expect(got.objects).toEqual([]);
    expect(got.same).toBe(0);
    expect(got.matchedByName).toBeUndefined();
  });
});

describe('parseUpdateStatus', () => {
  it('fills in what a short answer left out', () => {
    expect(parseUpdateStatus({ current: 'v1.14.1' })).toEqual({
      checked: false,
      current: 'v1.14.1',
      latest: undefined,
      available: false,
      url: undefined,
      command: undefined,
      reason: undefined,
    });
  });

  it('keeps the offer and the command apart from the versions', () => {
    const status = parseUpdateStatus({
      checked: true,
      current: 'v1.14.1',
      latest: 'v1.15.0',
      available: true,
      url: 'https://example.test/tag',
      command: 'curl -fsSL https://spinoza.tech/install.sh | sh',
    });

    expect(status.available).toBe(true);
    expect(status.latest).toBe('v1.15.0');
    expect(status.command).toContain('install.sh');
  });
});

describe('parseUpdateResult', () => {
  it('fills in what a short answer left out', () => {
    expect(parseUpdateResult({ current: 'v1.14.1' })).toEqual({
      updated: false,
      current: 'v1.14.1',
      latest: undefined,
      reason: undefined,
      command: undefined,
    });
  });

  it('keeps the outcome, the release and the command apart', () => {
    const result = parseUpdateResult({
      updated: true,
      current: 'v1.14.1',
      latest: 'v1.15.0',
      command: 'curl -fsSL https://spinoza.tech/install.sh | sh',
    });

    expect(result.updated).toBe(true);
    expect(result.latest).toBe('v1.15.0');
    expect(result.command).toContain('install.sh');
  });
});

describe('an object detail that carries gitops facts', () => {
  it('reads the deletion state and what holds it', () => {
    const detail = parseObjectDetail({
      terminating: true,
      finalizers: ['foregroundDeletion'],
    });

    expect(detail.terminating).toBe(true);
    expect(detail.finalizers).toEqual(['foregroundDeletion']);
  });

  it('leaves an object that is not going away unmarked', () => {
    const detail = parseObjectDetail({});

    expect(detail.terminating).toBeUndefined();
    expect(detail.finalizers).toBeUndefined();
  });

  it('reads a deletion with no finalizers left', () => {
    const detail = parseObjectDetail({ terminating: true });

    expect(detail.terminating).toBe(true);
    expect(detail.finalizers).toEqual([]);
  });

  it('reads the controller that owns it', () => {
    const detail = parseObjectDetail({
      managedBy: {
        controller: 'argocd',
        kind: 'Application',
        ref: {
          group: 'argoproj.io',
          version: 'v1alpha1',
          resource: 'applications',
          namespace: 'argocd',
          name: 'podinfo',
        },
      },
    });

    expect(detail.managedBy?.controller).toBe('argocd');
    expect(detail.managedBy?.ref.name).toBe('podinfo');
  });

  it('reads the source a flux object follows', () => {
    expect(parseObjectDetail({ source: 'GitRepository/flux-system' }).source).toBe(
      'GitRepository/flux-system',
    );
  });

  it('reads the consumers of a source', () => {
    const detail = parseObjectDetail({
      consumers: [
        {
          controller: 'flux',
          kind: 'Kustomization',
          ref: {
            group: 'kustomize.toolkit.fluxcd.io',
            version: 'v1',
            resource: 'kustomizations',
            namespace: 'flux-system',
            name: 'apps',
          },
        },
      ],
    });

    expect(detail.consumers?.[0].ref.name).toBe('apps');
  });

  it('leaves an unowned object without a chip', () => {
    expect(parseObjectDetail({}).managedBy).toBeUndefined();
  });
});

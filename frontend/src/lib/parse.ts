import type {
  ActionResult,
  ArgoActionResult,
  Category,
  ClusterOverview,
  Column,
  Comparison,
  Condition,
  ContainerState,
  DebugSession,
  DebugSupport,
  ExecSupport,
  LocalShell,
  FluxActionResult,
  FluxController,
  FluxDashboard,
  FluxOverview,
  FluxSync,
  FluxUsage,
  FluxGroup,
  FluxResource,
  Graph,
  GraphEdge,
  GraphNode,
  HelmActionResult,
  HelmChartVersions,
  HelmRelease,
  HelmReleaseDetail,
  HelmReleases,
  HelmRepoVersions,
  HelmResource,
  HelmRevision,
  HelmSupport,
  K8sEvent,
  MetricHistory,
  MetricPoint,
  Metrics,
  NodeSummary,
  ObjectDetail,
  ObjectPort,
  ObjectRef,
  OverviewEvent,
  OwnerRef,
  PodOutcome,
  PodSummary,
  PortForward,
  ResourceCatalog,
  ResourceCounts,
  ResourceDescriptor,
  DataEntry,
  ResourceUsage,
  Row,
} from './types';
import {
  CONTAINER_PHASES,
  EVENT_TYPES,
  FORWARD_STATES,
  GRAPH_EDGE_KINDS,
  GRAPH_NODE_CATEGORIES,
  READY_STATES,
} from './types';
import {
  asBoolean,
  asList,
  asNumber,
  asRecord,
  asString,
  listOf,
  numberMap,
  optionalNumberMap,
  oneOf,
  optionalBoolean,
  optionalListOf,
  optionalNumber,
  optionalString,
  recordMap,
  stringList,
  stringMap,
} from './wire';

export function parseColumn(item: Record<string, unknown>): Column {
  return { name: asString(item.name), render: optionalString(item.render) };
}

function parseContainerState(item: Record<string, unknown>): ContainerState {
  return {
    name: asString(item.name),
    state: oneOf(item.state, CONTAINER_PHASES, 'waiting'),
    reason: optionalString(item.reason),
    ready: asBoolean(item.ready),
    restarts: asNumber(item.restarts),
    init: asBoolean(item.init),
    ephemeral: optionalBoolean(item.ephemeral),
  };
}

export function parseRow(item: Record<string, unknown>): Row {
  return {
    uid: asString(item.uid),
    name: asString(item.name),
    namespace: asString(item.namespace),
    createdAt: asString(item.createdAt),
    cells: asList(item.cells).map(asString),
    containers: optionalListOf(item.containers, parseContainerState),
  };
}

function parseDescriptor(item: Record<string, unknown>): ResourceDescriptor {
  return {
    group: asString(item.group),
    version: asString(item.version),
    resource: asString(item.resource),
    kind: asString(item.kind),
    namespaced: asBoolean(item.namespaced),
    category: asString(item.category),
  };
}

function parseCategory(item: Record<string, unknown>): Category {
  return { name: asString(item.name), resources: listOf(item.resources, parseDescriptor) };
}

export function parseCatalog(body: unknown): ResourceCatalog {
  const item = asRecord(body);
  return { categories: listOf(item.categories, parseCategory), error: optionalString(item.error) };
}

function parseOwner(item: Record<string, unknown>): OwnerRef {
  return { kind: asString(item.kind), name: asString(item.name), uid: asString(item.uid) };
}

function parseCondition(item: Record<string, unknown>): Condition {
  return {
    type: asString(item.type),
    status: asString(item.status),
    reason: optionalString(item.reason),
    message: optionalString(item.message),
    updated: optionalString(item.updated),
  };
}

function parsePort(item: Record<string, unknown>): ObjectPort {
  return {
    name: optionalString(item.name),
    port: asNumber(item.port),
    protocol: optionalString(item.protocol),
  };
}

export function parseComparison(body: unknown): Comparison {
  const item = asRecord(body);
  return {
    left: asString(item.left),
    right: asString(item.right),
    leftContext: asString(item.leftContext),
    rightContext: asString(item.rightContext),
    identical: asBoolean(item.identical),
    missing: optionalString(item.missing),
  };
}

function parseDataEntry(item: Record<string, unknown>): DataEntry {
  return {
    key: asString(item.key),
    value: asString(item.value),
    bytes: asNumber(item.bytes),
    binary: optionalBoolean(item.binary),
  };
}

export function parseObjectDetail(body: unknown): ObjectDetail {
  const item = asRecord(body);
  const detail: ObjectDetail = {
    apiVersion: asString(item.apiVersion),
    kind: asString(item.kind),
    name: asString(item.name),
    namespace: asString(item.namespace),
    uid: asString(item.uid),
    createdAt: asString(item.createdAt),
    labels: stringMap(item.labels),
    annotations: stringMap(item.annotations),
    owners: optionalListOf(item.owners, parseOwner),
    conditions: optionalListOf(item.conditions, parseCondition),
    ports: optionalListOf(item.ports, parsePort),
    data: optionalListOf(item.data, parseDataEntry),
    yaml: asString(item.yaml),
  };
  if (Array.isArray(item.containers)) {
    detail.pod = { containers: asList(item.containers).map(asString) };
  }
  const event = asRecord(item.event);
  if (Object.keys(event).length > 0) {
    detail.event = {
      type: optionalString(event.type),
      reason: optionalString(event.reason),
      message: optionalString(event.message),
      object: optionalString(event.object),
      source: optionalString(event.source),
      count: optionalNumber(event.count),
      firstSeen: optionalString(event.firstSeen),
      lastSeen: optionalString(event.lastSeen),
    };
  }
  const replicas = optionalNumber(item.replicas);
  if (replicas !== undefined) {
    detail.workload = { replicas };
  }
  const schedulable = optionalBoolean(item.schedulable);
  if (schedulable !== undefined) {
    detail.node = { schedulable };
  }
  const suspended = optionalBoolean(item.suspended);
  const handledAt = optionalString(item.handledAt);
  if (suspended !== undefined || handledAt !== undefined) {
    detail.flux = { suspended: suspended === true, handledAt };
  }
  return detail;
}

export function parseEvents(body: unknown): K8sEvent[] {
  return listOf(body, (item) => ({
    type: oneOf(item.type, EVENT_TYPES, 'Normal'),
    reason: asString(item.reason),
    message: asString(item.message),
    source: asString(item.source),
    count: asNumber(item.count),
    firstSeen: asString(item.firstSeen),
    lastSeen: asString(item.lastSeen),
  }));
}

export function parseForward(item: Record<string, unknown>): PortForward {
  return {
    id: asString(item.id),
    kind: asString(item.kind),
    namespace: asString(item.namespace),
    name: asString(item.name),
    pod: optionalString(item.pod),
    remotePort: asNumber(item.remotePort),
    localPort: asNumber(item.localPort),
    state: oneOf(item.state, FORWARD_STATES, 'failed'),
    error: optionalString(item.error),
    startedAt: asString(item.startedAt),
  };
}

export function parseForwards(body: unknown): PortForward[] {
  return listOf(body, parseForward);
}

function parseFluxResource(item: Record<string, unknown>): FluxResource {
  return {
    kind: asString(item.kind),
    group: asString(item.group),
    version: asString(item.version),
    resource: asString(item.resource),
    name: asString(item.name),
    namespace: asString(item.namespace),
    ready: oneOf(item.ready, READY_STATES, 'Unknown'),
    suspended: asBoolean(item.suspended),
    revision: asString(item.revision),
    latest: optionalString(item.latest),
    outdated: optionalBoolean(item.outdated),
    source: asString(item.source),
    message: asString(item.message),
    createdAt: asString(item.createdAt),
  };
}

function parseFluxGroup(item: Record<string, unknown>): FluxGroup {
  return {
    name: asString(item.name),
    ready: asNumber(item.ready),
    reporting: asNumber(item.reporting),
    total: asNumber(item.total),
    resources: listOf(item.resources, parseFluxResource),
  };
}

export function parseFluxDashboard(body: unknown): FluxDashboard {
  const item = asRecord(body);
  return { groups: listOf(item.groups, parseFluxGroup), error: optionalString(item.error) };
}

function parseFluxController(item: Record<string, unknown>): FluxController {
  return {
    name: asString(item.name),
    version: asString(item.version),
    ready: item.ready === true,
    replicas: asNumber(item.replicas),
    wanted: asNumber(item.wanted),
    namespace: asString(item.namespace),
  };
}

function parseFluxSync(raw: unknown): FluxSync {
  const item = asRecord(raw);
  return {
    namespace: asString(item.namespace),
    name: asString(item.name),
    kind: asString(item.kind),
    source: asString(item.source),
    url: asString(item.url),
    ref: asString(item.ref),
    path: asString(item.path),
    revision: asString(item.revision),
    ready: item.ready === true,
  };
}

function parseFluxUsage(raw: unknown): FluxUsage {
  const item = asRecord(raw);
  return {
    cpuMilli: asNumber(item.cpuMilli),
    memoryMi: asNumber(item.memoryMi),
    cpuRequestMilli: asNumber(item.cpuRequestMilli),
    memRequestMi: asNumber(item.memRequestMi),
    cpuLimitMilli: asNumber(item.cpuLimitMilli),
    memLimitMi: asNumber(item.memLimitMi),
    known: item.known === true,
  };
}

export function parseFluxOverview(body: unknown): FluxOverview {
  const item = asRecord(body);
  return {
    ready: item.ready === true,
    summary: asString(item.summary),
    namespace: asString(item.namespace),
    kubernetes: asString(item.kubernetes),
    nodes: asNumber(item.nodes),
    operator: optionalString(item.operator),
    distribution: optionalString(item.distribution),
    controllers: listOf(item.controllers, parseFluxController),
    sync: parseFluxSync(item.sync),
    usage: parseFluxUsage(item.usage),
    error: optionalString(item.error),
  };
}

export function parseFluxActionResult(body: unknown): FluxActionResult {
  const item = asRecord(body);
  return { action: asString(item.action), requestedAt: optionalString(item.requestedAt) };
}

export function parseArgoActionResult(body: unknown): ArgoActionResult {
  return { action: asString(asRecord(body).action) };
}

function parsePodOutcome(item: Record<string, unknown>): PodOutcome {
  return {
    namespace: asString(item.namespace),
    name: asString(item.name),
    outcome: asString(item.outcome),
    reason: optionalString(item.reason),
  };
}

export function parseActionResult(body: unknown): ActionResult {
  const item = asRecord(body);
  return {
    action: asString(item.action),
    message: asString(item.message),
    dryRun: optionalBoolean(item.dryRun),
    pods: optionalListOf(item.pods, parsePodOutcome),
  };
}

function parseGraphNode(item: Record<string, unknown>): GraphNode {
  return {
    id: asString(item.id),
    kind: asString(item.kind),
    group: asString(item.group),
    version: asString(item.version),
    resource: asString(item.resource),
    name: asString(item.name),
    namespace: asString(item.namespace),
    status: asString(item.status),
    ready: oneOf(item.ready, READY_STATES, 'Unknown'),
    category: oneOf(item.category, GRAPH_NODE_CATEGORIES, 'managed'),
  };
}

function parseGraphEdge(item: Record<string, unknown>): GraphEdge {
  return {
    from: asString(item.from),
    to: asString(item.to),
    kind: oneOf(item.kind, GRAPH_EDGE_KINDS, 'manages'),
  };
}

export function parseGraph(body: unknown): Graph {
  const item = asRecord(body);
  return {
    nodes: listOf(item.nodes, parseGraphNode),
    edges: listOf(item.edges, parseGraphEdge),
    error: optionalString(item.error),
  };
}

function parseUsage(item: Record<string, unknown>): ResourceUsage {
  return {
    cpuMilli: asNumber(item.cpuMilli),
    memoryMi: asNumber(item.memoryMi),
    cpuPercent: asNumber(item.cpuPercent),
    memPercent: asNumber(item.memPercent),
  };
}

export function parseMetrics(body: unknown): Metrics {
  const item = asRecord(body);
  return {
    pods: recordMap(item.pods, parseUsage),
    nodes: recordMap(item.nodes, parseUsage),
    error: optionalString(item.error),
  };
}

function parseMetricPoint(item: Record<string, unknown>): MetricPoint {
  return { at: asNumber(item.at), value: asNumber(item.value) };
}

export function parseMetricHistory(body: unknown): MetricHistory {
  const item = asRecord(body);
  return {
    namespace: asString(item.namespace),
    pod: asString(item.pod),
    source: optionalString(item.source),
    cpu: listOf(item.cpu, parseMetricPoint),
    memory: listOf(item.memory, parseMetricPoint),
  };
}

export function parseCounts(body: unknown): ResourceCounts {
  const item = asRecord(body);
  return {
    counts: numberMap(item.counts),
    failing: optionalNumberMap(item.failing),
    errors: stringMap(item.errors),
  };
}

export function parseExecSupport(body: unknown): ExecSupport {
  const item = asRecord(body);
  return {
    namespace: asString(item.namespace),
    pod: asString(item.pod),
    container: asString(item.container),
    image: optionalString(item.image),
    shell: oneOf(item.shell, ['unknown', 'present', 'absent'] as const, 'unknown'),
  };
}

export function parseLocalShell(body: unknown): LocalShell {
  const item = asRecord(body);
  return {
    available: item.available === true,
    reason: optionalString(item.reason),
  };
}

export function parseDebugSession(body: unknown): DebugSession {
  const item = asRecord(body);
  return {
    container: asString(item.container),
    created: asBoolean(item.created),
    image: asString(item.image),
    profile: asString(item.profile),
    target: optionalString(item.target),
  };
}

export function parseDebugSupport(body: unknown): DebugSupport {
  const item = asRecord(body);
  return {
    namespace: asString(item.namespace),
    pod: optionalString(item.pod),
    allowed: asBoolean(item.allowed),
    reason: optionalString(item.reason),
    image: asString(item.image),
  };
}

function parseNodeSummary(item: Record<string, unknown>): NodeSummary {
  return {
    total: asNumber(item.total),
    ready: asNumber(item.ready),
    unschedulable: asNumber(item.unschedulable),
    cpuAllocatableMilli: asNumber(item.cpuAllocatableMilli),
    cpuUsedMilli: asNumber(item.cpuUsedMilli),
    memAllocatableMi: asNumber(item.memAllocatableMi),
    memUsedMi: asNumber(item.memUsedMi),
    usageKnown: asBoolean(item.usageKnown),
  };
}

function parsePodSummary(item: Record<string, unknown>): PodSummary {
  return {
    total: asNumber(item.total),
    running: asNumber(item.running),
    pending: asNumber(item.pending),
    failed: asNumber(item.failed),
    succeeded: asNumber(item.succeeded),
    known: asBoolean(item.known),
  };
}

function parseOverviewEvent(item: Record<string, unknown>): OverviewEvent {
  return {
    namespace: asString(item.namespace),
    object: asString(item.object),
    reason: asString(item.reason),
    message: asString(item.message),
    count: asNumber(item.count),
    lastSeen: asString(item.lastSeen),
  };
}

export function parseClusterOverview(body: unknown): ClusterOverview {
  const item = asRecord(body);
  return {
    version: asString(item.version),
    nodes: parseNodeSummary(asRecord(item.nodes)),
    pods: parsePodSummary(asRecord(item.pods)),
    warnings: listOf(item.warnings, parseOverviewEvent),
    error: optionalString(item.error),
  };
}

function parseObjectRef(item: Record<string, unknown>): ObjectRef {
  return {
    group: asString(item.group),
    version: asString(item.version),
    resource: asString(item.resource),
    namespace: asString(item.namespace),
    name: asString(item.name),
  };
}

function optionalObjectRef(value: unknown): ObjectRef | undefined {
  if (value === null) {
    return undefined;
  }
  if (typeof value !== 'object') {
    return undefined;
  }
  return parseObjectRef(asRecord(value));
}

function parseHelmRelease(item: Record<string, unknown>): HelmRelease {
  return {
    name: asString(item.name),
    namespace: asString(item.namespace),
    chart: asString(item.chart),
    chartVersion: asString(item.chartVersion),
    appVersion: asString(item.appVersion),
    latest: optionalString(item.latest),
    outdated: optionalBoolean(item.outdated),
    revision: asNumber(item.revision),
    status: asString(item.status),
    updated: asString(item.updated),
    description: optionalString(item.description),
    fluxRef: optionalObjectRef(item.fluxRef),
  };
}

function parseHelmRevision(item: Record<string, unknown>): HelmRevision {
  return {
    revision: asNumber(item.revision),
    status: asString(item.status),
    chartVersion: asString(item.chartVersion),
    appVersion: asString(item.appVersion),
    updated: asString(item.updated),
    description: optionalString(item.description),
  };
}

function parseHelmResource(item: Record<string, unknown>): HelmResource {
  return {
    apiVersion: asString(item.apiVersion),
    kind: asString(item.kind),
    name: asString(item.name),
    namespace: optionalString(item.namespace),
    group: optionalString(item.group),
    version: optionalString(item.version),
    resource: optionalString(item.resource),
  };
}

export function parseHelmReleaseDetail(body: unknown): HelmReleaseDetail {
  const item = asRecord(body);
  return {
    release: parseHelmRelease(asRecord(item.release)),
    driver: asString(item.driver),
    firstDeployed: optionalString(item.firstDeployed),
    values: asString(item.values),
    notes: asString(item.notes),
    manifest: asString(item.manifest),
    resources: listOf(item.resources, parseHelmResource),
    history: listOf(item.history, parseHelmRevision),
    error: optionalString(item.error),
  };
}

export function parseHelmSupport(body: unknown): HelmSupport {
  const item = asRecord(body);
  return {
    available: asBoolean(item.available),
    reason: optionalString(item.reason),
    binary: asString(item.binary),
  };
}

export function parseHelmActionResult(body: unknown): HelmActionResult {
  const item = asRecord(body);
  return {
    action: asString(item.action),
    message: asString(item.message),
    revision: optionalNumber(item.revision),
    dryRun: optionalBoolean(item.dryRun),
    manifest: optionalString(item.manifest),
  };
}

function parseHelmRepoVersions(item: Record<string, unknown>): HelmRepoVersions {
  return {
    name: optionalString(item.name),
    url: asString(item.url),
    oci: optionalBoolean(item.oci),
    versions: stringList(item.versions),
  };
}

export function parseHelmChartVersions(body: unknown): HelmChartVersions {
  const item = asRecord(body);
  return {
    chart: asString(item.chart),
    repos: listOf(item.repos, parseHelmRepoVersions),
    error: optionalString(item.error),
  };
}

export function parseHelmReleases(body: unknown): HelmReleases {
  const item = asRecord(body);
  return {
    releases: listOf(item.releases, parseHelmRelease),
    error: optionalString(item.error),
  };
}

import type {
  ActionResult,
  Category,
  Column,
  Condition,
  ContainerState,
  DebugSession,
  DebugSupport,
  ExecSupport,
  FluxActionResult,
  FluxDashboard,
  FluxGroup,
  FluxResource,
  Graph,
  GraphEdge,
  GraphNode,
  K8sEvent,
  MetricHistory,
  MetricPoint,
  Metrics,
  ObjectDetail,
  ObjectPort,
  OwnerRef,
  PodOutcome,
  PortForward,
  ResourceCatalog,
  ResourceDescriptor,
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
  oneOf,
  optionalBoolean,
  optionalListOf,
  optionalNumber,
  optionalString,
  recordMap,
  stringMap,
} from './wire';

export function parseColumn(item: Record<string, unknown>): Column {
  return { name: asString(item.name), render: optionalString(item.render) };
}

export function parseContainerState(item: Record<string, unknown>): ContainerState {
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

export function parseDescriptor(item: Record<string, unknown>): ResourceDescriptor {
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
    yaml: asString(item.yaml),
  };
  if (Array.isArray(item.containers)) {
    detail.pod = { containers: asList(item.containers).map(asString) };
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

export function parseFluxActionResult(body: unknown): FluxActionResult {
  const item = asRecord(body);
  return { action: asString(item.action), requestedAt: optionalString(item.requestedAt) };
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

export function parseCounts(body: unknown): Record<string, number> {
  return numberMap(asRecord(body).counts);
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

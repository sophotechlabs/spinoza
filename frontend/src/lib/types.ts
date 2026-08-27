export interface ResourceDescriptor {
  group: string;
  version: string;
  resource: string;
  kind: string;
  namespaced: boolean;
  category: string;
}

export interface Category {
  name: string;
  resources: ResourceDescriptor[];
}

export interface Column {
  name: string;
  render?: string;
}

export const CONTAINER_PHASES = ['running', 'waiting', 'terminated'] as const;

export type ContainerPhase = (typeof CONTAINER_PHASES)[number];

export interface ContainerState {
  name: string;
  state: ContainerPhase;
  reason?: string;
  ready: boolean;
  restarts: number;
  init: boolean;
  ephemeral?: boolean;
}

export interface Row {
  uid: string;
  name: string;
  namespace: string;
  createdAt: string;
  cells: string[];
  containers?: ContainerState[];
}

export interface ObjectRef {
  group: string;
  version: string;
  resource: string;
  namespace: string;
  name: string;
}

export interface OwnerRef {
  kind: string;
  name: string;
  uid: string;
}

export interface Condition {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  updated?: string;
}

export interface PodDetail {
  containers: string[];
}

export interface WorkloadDetail {
  replicas: number;
}

export interface NodeDetail {
  schedulable: boolean;
}

export interface FluxDetail {
  suspended: boolean;
  handledAt?: string;
}

export interface ObjectEvent {
  type?: string;
  reason?: string;
  message?: string;
  object?: string;
  source?: string;
  count?: number;
  firstSeen?: string;
  lastSeen?: string;
}

export interface Comparison {
  left: string;
  right: string;
  leftContext: string;
  rightContext: string;
  identical: boolean;
  missing?: string;
}

export const VERDICTS = ['same', 'differs', 'onlyHere', 'onlyThere'] as const;

export type Verdict = (typeof VERDICTS)[number];

export interface KindDiff {
  namespace?: string;
  name: string;
  verdict: Verdict;
  lines?: number;
}

export interface KindComparison {
  resource: string;
  leftContext: string;
  rightContext: string;
  namespace?: string;
  objects: KindDiff[];
  same: number;
  differs: number;
  onlyHere: number;
  onlyThere: number;
  matchedByName?: boolean;
}

export interface NodeShellSupport {
  node: string;
  enabled: boolean;
  allowed: boolean;
  reason?: string;
  image: string;
  namespace: string;
}

export interface DataEntry {
  key: string;
  value: string;
  bytes: number;
  binary?: boolean;
}

export interface ObjectDetail {
  apiVersion: string;
  kind: string;
  name: string;
  namespace: string;
  uid: string;
  createdAt: string;
  labels?: Record<string, string>;
  annotations?: Record<string, string>;
  owners?: OwnerRef[];
  conditions?: Condition[];
  ports?: ObjectPort[];
  event?: ObjectEvent;
  data?: DataEntry[];
  yaml: string;
  pod?: PodDetail;
  workload?: WorkloadDetail;
  node?: NodeDetail;
  flux?: FluxDetail;
}

export interface ObjectPort {
  name?: string;
  port: number;
  protocol?: string;
}

export const FORWARD_STATES = ['running', 'failed'] as const;

export type ForwardState = (typeof FORWARD_STATES)[number];

export interface PortForward {
  id: string;
  kind: string;
  namespace: string;
  name: string;
  pod?: string;
  remotePort: number;
  localPort: number;
  state: ForwardState;
  error?: string;
  startedAt: string;
}

// What the cluster says this user may not do to the selected object. Anything
// absent is either permitted or unanswerable, and both mean: let them try.
export interface Refusal {
  capability: string;
  reason: string;
}

export interface Access {
  refused: Refusal[];
}

// One question about a whole selection: what would stand in the way of this
// capability, row by row.
export interface AccessQuery {
  capability: string;
  refs: ObjectRef[];
}

export interface BulkAccess {
  refused: RowRefusal[];
}

// at is the row's place in the list that was asked about.
export interface RowRefusal {
  at: number;
  reason: string;
}

export const EVENT_TYPES = ['Normal', 'Warning'] as const;

export type EventType = (typeof EVENT_TYPES)[number];

export interface K8sEvent {
  type: EventType;
  reason: string;
  message: string;
  source: string;
  count: number;
  firstSeen: string;
  lastSeen: string;
}

export interface LogRequest {
  namespace: string;
  name: string;
  container: string;
  tailLines: number;
  follow: boolean;
  group?: string;
  version?: string;
  resource?: string;
}

export interface Failure {
  message: string;
}

export interface RowFilter {
  field: string;
  value: string;
}

export type ClientMsg =
  | {
      type: 'subscribe';
      subId: string;
      group: string;
      version: string;
      resource: string;
      namespace: string;
      limit: number;
      filters?: RowFilter[];
    }
  | { type: 'unsubscribe'; subId: string }
  | { type: 'more'; subId: string; limit: number }
  | {
      type: 'logs-subscribe';
      subId: string;
      namespace: string;
      name: string;
      container: string;
      tailLines: number;
      follow: boolean;
      group?: string;
      version?: string;
      resource?: string;
    }
  | { type: 'logs-unsubscribe'; subId: string };

export type ServerMsg =
  | {
      type: 'snapshot';
      subId: string;
      columns: Column[];
      namespaced: boolean;
      rows: Row[];
      total: number;
      limit: number;
    }
  | { type: 'added'; subId: string; row: Row }
  | { type: 'modified'; subId: string; row: Row }
  | { type: 'deleted'; subId: string; uid: string }
  | { type: 'batch'; subId: string; changes: ServerMsg[] }
  | { type: 'log'; subId: string; lines: string[]; source?: string }
  | { type: 'log-open'; subId: string; attached: number; matched: number }
  | { type: 'log-end'; subId: string }
  | { type: 'context'; subId: string; context: string }
  | { type: 'cluster'; subId: string; reachable: boolean; reason?: string }
  | { type: 'error'; subId: string; message: string };

export const VIEWS = [
  'resources',
  'cluster',
  'helm',
  'gitops',
  'flux-list',
  'flux-roles',
  'argo-apps',
  'argo-graph',
  'argo-list',
] as const;

export type View = (typeof VIEWS)[number];

export const FLUX_VIEWS: View[] = ['flux-roles', 'gitops', 'flux-list'];

export const ARGO_VIEWS: View[] = ['argo-apps', 'argo-graph', 'argo-list'];

export interface ArgoApp {
  kind: string;
  group: string;
  version: string;
  resource: string;
  name: string;
  namespace: string;
  project: string;
  sync: string;
  health: string;
  revision: string;
  repo: string;
  path: string;
  destination: string;
  message: string;
  owner?: string;
  createdAt: string;
}

export interface ArgoDashboard {
  apps: ArgoApp[];
  applicationSets: ArgoApp[];
  projects: ArgoApp[];
  error?: string;
}

export interface FluxController {
  name: string;
  version: string;
  ready: boolean;
  replicas: number;
  wanted: number;
  namespace: string;
}

export interface FluxSync {
  namespace: string;
  name: string;
  kind: string;
  source: string;
  url: string;
  ref: string;
  path: string;
  revision: string;
  ready: boolean;
}

export interface FluxUsage {
  cpuMilli: number;
  memoryMi: number;
  cpuRequestMilli: number;
  memRequestMi: number;
  cpuLimitMilli: number;
  memLimitMi: number;
  known: boolean;
}

export interface FluxOverview {
  ready: boolean;
  summary: string;
  namespace: string;
  kubernetes: string;
  nodes: number;
  operator?: string;
  distribution?: string;
  controllers: FluxController[];
  sync: FluxSync;
  usage: FluxUsage;
  error?: string;
}

export interface FluxResource {
  kind: string;
  group: string;
  version: string;
  resource: string;
  name: string;
  namespace: string;
  ready: ReadyState;
  suspended: boolean;
  revision: string;
  latest?: string;
  outdated?: boolean;
  source: string;
  message: string;
  createdAt: string;
}

export interface ContextRef {
  kubeconfig: string;
  name: string;
}

export interface KubeContext {
  cluster: string;
  name: string;
  namespace?: string;
}

export interface Kubeconfig {
  contexts: KubeContext[];
  error?: string;
  label: string;
  path: string;
  removable: boolean;
}

export type Protection = 'protected' | 'open' | 'unknown';

export interface ContextList {
  current: ContextRef;
  error?: string;
  kubeconfigs: Kubeconfig[];
  protection: Protection;
}

export interface FilePicker {
  available: boolean;
  reason?: string;
}

export interface Namespaces {
  names: string[];
  error?: string;
}

export interface SearchHit {
  group: string;
  version: string;
  resource: string;
  kind: string;
  namespace: string;
  name: string;
}

export interface SearchResults {
  hits: SearchHit[];
  truncated: boolean;
  errors?: Record<string, string>;
}

export interface ViewState {
  window: boolean;
  hidden: boolean;
}

export interface ViewSwitch {
  switched: boolean;
  reason?: string;
}

export interface Settings {
  values: Record<string, string>;
}

export interface LocalShell {
  available: boolean;
  reason?: string;
}

export interface TerminalSize {
  cols: number;
  rows: number;
}

export interface PickedFile {
  path: string;
}

export interface ResourceCounts {
  counts: Record<string, number>;
  failing?: Record<string, number>;
  errors?: Record<string, string>;
}

export interface ResourceCatalog {
  categories: Category[];
  error?: string;
}

export interface NodeSummary {
  total: number;
  ready: number;
  unschedulable: number;
  cpuAllocatableMilli: number;
  cpuUsedMilli: number;
  memAllocatableMi: number;
  memUsedMi: number;
  usageKnown: boolean;
}

export interface PodSummary {
  total: number;
  running: number;
  pending: number;
  failed: number;
  succeeded: number;
  known: boolean;
}

export interface OverviewEvent {
  namespace: string;
  object: string;
  reason: string;
  message: string;
  count: number;
  lastSeen: string;
}

export interface ClusterOverview {
  version: string;
  nodes: NodeSummary;
  pods: PodSummary;
  warnings: OverviewEvent[];
  error?: string;
}

export interface ReleaseRef {
  namespace: string;
  name: string;
}

export interface HelmRelease {
  name: string;
  namespace: string;
  chart: string;
  chartVersion: string;
  appVersion: string;
  latest?: string;
  outdated?: boolean;
  revision: number;
  status: string;
  updated: string;
  description?: string;
  fluxRef?: ObjectRef;
}

export interface HelmReleases {
  releases: HelmRelease[];
  error?: string;
}

export interface HelmRevision {
  revision: number;
  status: string;
  chartVersion: string;
  appVersion: string;
  updated: string;
  description?: string;
}

export interface HelmResource {
  apiVersion: string;
  kind: string;
  name: string;
  namespace?: string;
  group?: string;
  version?: string;
  resource?: string;
}

export interface HelmReleaseDetail {
  release: HelmRelease;
  driver: string;
  firstDeployed?: string;
  values: string;
  notes: string;
  manifest: string;
  resources: HelmResource[];
  history: HelmRevision[];
  error?: string;
}

export interface HelmSupport {
  available: boolean;
  reason?: string;
  binary: string;
}

export interface HelmActionResult {
  action: string;
  message: string;
  revision?: number;
  dryRun?: boolean;
  manifest?: string;
}

export interface HelmRepoVersions {
  name?: string;
  url: string;
  oci?: boolean;
  versions: string[];
}

export interface HelmChartVersions {
  chart: string;
  repos: HelmRepoVersions[];
  error?: string;
}

export interface HelmChartHit {
  chart: string;
  version: string;
  description?: string;
  repo?: string;
  url: string;
}

export interface HelmChartSearch {
  query: string;
  hits: HelmChartHit[];
  truncated?: boolean;
  error?: string;
}

export interface HelmChartValues {
  chart: string;
  version: string;
  values: string;
}

export interface FluxGroup {
  name: string;
  ready: number;
  reporting: number;
  total: number;
  resources: FluxResource[];
}

export interface FluxDashboard {
  groups: FluxGroup[];
  error?: string;
}

export interface ResourceUsage {
  cpuMilli: number;
  memoryMi: number;
  cpuPercent: number;
  memPercent: number;
  // What the node has to give. Only a node has a ceiling of its own, so these
  // stay zero for a pod.
  cpuAllocatableMilli: number;
  memAllocatableMi: number;
}

export interface Metrics {
  pods: Record<string, ResourceUsage>;
  nodes: Record<string, ResourceUsage>;
  error?: string;
}

export const GRAPH_NODE_CATEGORIES = ['source', 'applier', 'app', 'managed'] as const;

export type GraphNodeCategory = (typeof GRAPH_NODE_CATEGORIES)[number];

export interface GraphNode {
  id: string;
  kind: string;
  group: string;
  version: string;
  resource: string;
  name: string;
  namespace: string;
  status: string;
  ready: ReadyState;
  category: GraphNodeCategory;
}

export const READY_STATES = ['True', 'False', 'Unknown', ''] as const;

export type ReadyState = (typeof READY_STATES)[number];

export const GRAPH_EDGE_KINDS = ['source', 'dependsOn', 'manages'] as const;

export type GraphEdgeKind = (typeof GRAPH_EDGE_KINDS)[number];

export interface GraphEdge {
  from: string;
  to: string;
  kind: GraphEdgeKind;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
  error?: string;
}

export interface ExecTarget {
  namespace: string;
  pod: string;
  container: string;
}

export type ShellState = 'unknown' | 'present' | 'absent';

export interface ExecSupport {
  namespace: string;
  pod: string;
  container: string;
  image?: string;
  shell: ShellState;
}

export interface FluxActionResult {
  action: string;
  requestedAt?: string;
}

export interface ArgoActionResult {
  action: string;
}

export interface PodOutcome {
  namespace: string;
  name: string;
  outcome: string;
  reason?: string;
}

export interface ActionResult {
  action: string;
  message: string;
  dryRun?: boolean;
  pods?: PodOutcome[];
}

export interface DebugSession {
  container: string;
  created: boolean;
  image: string;
  profile: string;
  target?: string;
}

export interface DebugSupport {
  namespace: string;
  pod?: string;
  allowed: boolean;
  reason?: string;
  image: string;
}

export interface MetricPoint {
  at: number;
  value: number;
}

export interface MetricHistory {
  namespace: string;
  pod: string;
  source?: string;
  sampled?: boolean;
  since?: number;
  cpu: MetricPoint[];
  memory: MetricPoint[];
}

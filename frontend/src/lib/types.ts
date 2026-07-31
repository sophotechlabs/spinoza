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

export interface ContainerState {
  name: string;
  state: string;
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
  containers?: string[];
  suspended?: boolean;
  handledAt?: string;
  ports?: ObjectPort[];
  yaml: string;
}

export interface ObjectPort {
  name?: string;
  port: number;
  protocol?: string;
}

export interface PortForward {
  id: string;
  kind: string;
  namespace: string;
  name: string;
  pod?: string;
  remotePort: number;
  localPort: number;
  state: string;
  error?: string;
  startedAt: string;
}

export interface K8sEvent {
  type: string;
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
}

export type ClientMsg =
  | {
      type: 'subscribe';
      subId: string;
      group: string;
      version: string;
      resource: string;
      namespace: string;
    }
  | { type: 'unsubscribe'; subId: string }
  | {
      type: 'logs-subscribe';
      subId: string;
      namespace: string;
      name: string;
      container: string;
      tailLines: number;
      follow: boolean;
    }
  | { type: 'logs-unsubscribe'; subId: string };

export type ServerMsg =
  | { type: 'snapshot'; subId: string; columns: Column[]; namespaced: boolean; rows: Row[] }
  | { type: 'added'; subId: string; row: Row }
  | { type: 'modified'; subId: string; row: Row }
  | { type: 'deleted'; subId: string; uid: string }
  | { type: 'log'; subId: string; lines: string[] }
  | { type: 'log-end'; subId: string }
  | { type: 'error'; subId: string; message: string };

export type View = 'resources' | 'gitops' | 'flux-list' | 'flux-overview' | 'flux-roles';

export interface FluxResource {
  kind: string;
  group: string;
  version: string;
  resource: string;
  name: string;
  namespace: string;
  ready: string;
  suspended: boolean;
  revision: string;
  latest?: string;
  outdated?: boolean;
  source: string;
  message: string;
  createdAt: string;
}

export interface ResourceCatalog {
  categories: Category[];
  error?: string;
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
}

export interface ResourceUsage {
  cpuMilli: number;
  memoryMi: number;
  cpuPercent: number;
  memPercent: number;
}

export interface Metrics {
  pods: Record<string, ResourceUsage>;
  nodes: Record<string, ResourceUsage>;
}

export type GraphNodeCategory = 'source' | 'applier' | 'app' | 'managed';

export interface GraphNode {
  id: string;
  kind: string;
  group: string;
  version: string;
  resource: string;
  name: string;
  namespace: string;
  status: string;
  category: GraphNodeCategory;
}

export type GraphEdgeKind = 'source' | 'dependsOn' | 'manages';

export interface GraphEdge {
  from: string;
  to: string;
  kind: GraphEdgeKind;
}

export interface Graph {
  nodes: GraphNode[];
  edges: GraphEdge[];
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

export interface DebugSession {
  container: string;
  created: boolean;
  image: string;
  profile: string;
}

export interface DebugSupport {
  namespace: string;
  allowed: boolean;
  reason?: string;
}

export interface MetricPoint {
  at: number;
  value: number;
}

export interface MetricHistory {
  namespace: string;
  pod: string;
  source?: string;
  cpu: MetricPoint[];
  memory: MetricPoint[];
}

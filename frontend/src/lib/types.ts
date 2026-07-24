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
}

export interface Row {
  uid: string;
  name: string;
  namespace: string;
  createdAt: string;
  cells: string[];
  containers?: ContainerState[];
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
  | { type: 'unsubscribe'; subId: string };

export type ServerMsg =
  | { type: 'snapshot'; subId: string; columns: Column[]; namespaced: boolean; rows: Row[] }
  | { type: 'added'; subId: string; row: Row }
  | { type: 'modified'; subId: string; row: Row }
  | { type: 'deleted'; subId: string; uid: string }
  | { type: 'error'; subId: string; message: string };

export type View = 'resources' | 'gitops' | 'flux';

export interface FluxResource {
  kind: string;
  name: string;
  namespace: string;
  ready: string;
  suspended: boolean;
  revision: string;
  source: string;
  message: string;
  createdAt: string;
}

export interface FluxGroup {
  name: string;
  ready: number;
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

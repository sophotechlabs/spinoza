import dagre from '@dagrejs/dagre';
import type { Graph as DagreGraph, EdgeLabel, GraphLabel, NodeLabel } from '@dagrejs/dagre';
import type { Edge, Node } from '@xyflow/react';
import type { TrafficEdge, TrafficGraph, TrafficNode } from './types';

const NODE_WIDTH = 180;
const NODE_HEIGHT = 44;
const RANK_SEPARATION = 90;
const NODE_SEPARATION = 28;

const NODE_CLASS =
  'rounded border px-2 py-1 text-[11px] font-mono truncate border-edge-emphasis bg-surface-active text-fg-strong';

export const EDGE_FLOW_STROKE = 'var(--graph-edge-flow)';
export const EDGE_DROP_STROKE = 'var(--graph-edge-drop)';

interface TrafficNodeData {
  label: string;
  node: TrafficNode;
  [key: string]: unknown;
}

export type TrafficFlowNode = Node<TrafficNodeData>;

export interface TrafficFlow {
  nodes: TrafficFlowNode[];
  edges: Edge[];
}

type LayoutGraph = DagreGraph<GraphLabel, NodeLabel, EdgeLabel>;

export function perSecond(value: number): string {
  if (value >= 100) {
    return String(Math.round(value));
  }
  if (value >= 10) {
    return value.toFixed(1);
  }
  return value.toFixed(2);
}

export function edgeLabel(edge: TrafficEdge): string {
  const flows = `${perSecond(edge.rate)}/s`;
  if (edge.dropped === 0) {
    return flows;
  }
  return `${flows} · ${perSecond(edge.dropped)} dropped`;
}

function edgeStroke(edge: TrafficEdge): string {
  if (edge.dropped > 0) {
    return EDGE_DROP_STROKE;
  }
  return EDGE_FLOW_STROKE;
}

function layoutPosition(laid: NodeLabel): { x: number; y: number } {
  let x = 0;
  if (laid.x !== undefined) {
    x = laid.x - NODE_WIDTH / 2;
  }
  let y = 0;
  if (laid.y !== undefined) {
    y = laid.y - NODE_HEIGHT / 2;
  }
  return { x, y };
}

export function nodeLabel(node: TrafficNode): string {
  if (node.workload === '') {
    return node.namespace;
  }
  return node.id;
}

function toFlowNode(g: LayoutGraph, node: TrafficNode): TrafficFlowNode {
  return {
    id: node.id,
    position: layoutPosition(g.node(node.id)),
    data: { label: nodeLabel(node), node },
    className: NODE_CLASS,
  };
}

function toFlowEdge(edge: TrafficEdge, dense: boolean): Edge {
  const drawn: Edge = {
    id: `${edge.from}->${edge.to}`,
    source: edge.from,
    target: edge.to,
    animated: !dense,
    style: { stroke: edgeStroke(edge) },
  };
  if (dense) {
    return drawn;
  }
  return { ...drawn, label: edgeLabel(edge), labelStyle: { fontSize: 10 } };
}

export function sameTrafficShape(a: TrafficGraph, b: TrafficGraph): boolean {
  if (a.nodes.length !== b.nodes.length) {
    return false;
  }
  if (!a.nodes.every((node, index) => node.id === b.nodes[index].id)) {
    return false;
  }
  if (a.edges.length !== b.edges.length) {
    return false;
  }
  return a.edges.every((edge, index) => sameEnds(edge, b.edges[index]));
}

export function sameTraffic(a: TrafficGraph, b: TrafficGraph): boolean {
  if (a.error !== b.error) {
    return false;
  }
  if (!sameTrafficShape(a, b)) {
    return false;
  }
  return a.edges.every((edge, index) => sameRates(edge, b.edges[index]));
}

function sameEnds(a: TrafficEdge, b: TrafficEdge): boolean {
  if (a.from !== b.from) {
    return false;
  }
  return a.to === b.to;
}

function sameRates(a: TrafficEdge, b: TrafficEdge): boolean {
  if (a.rate !== b.rate) {
    return false;
  }
  return a.dropped === b.dropped;
}

export function restyleTraffic(flow: TrafficFlow, graph: TrafficGraph): TrafficFlow {
  return {
    nodes: flow.nodes,
    edges: busiest(graph.edges).map((edge) => toFlowEdge(edge, graph.edges.length > MAX_EDGES)),
  };
}

export const MAX_EDGES = 240;

function busiest(edges: TrafficEdge[]): TrafficEdge[] {
  if (edges.length <= MAX_EDGES) {
    return edges;
  }
  const byRate = [...edges].sort((left, right) => right.rate - left.rate);
  return byRate.slice(0, MAX_EDGES);
}

export function toTrafficFlow(graph: TrafficGraph): TrafficFlow {
  const g: LayoutGraph = new dagre.graphlib.Graph<GraphLabel, NodeLabel, EdgeLabel>();
  g.setGraph({ rankdir: 'LR', ranksep: RANK_SEPARATION, nodesep: NODE_SEPARATION });
  g.setDefaultEdgeLabel(() => ({}));
  for (const node of graph.nodes) {
    g.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }
  const drawn = busiest(graph.edges);
  const dense = graph.edges.length > MAX_EDGES;
  for (const edge of drawn) {
    g.setEdge(edge.from, edge.to);
  }
  dagre.layout(g);
  return {
    nodes: graph.nodes.map((node) => toFlowNode(g, node)),
    edges: drawn.map((edge) => toFlowEdge(edge, dense)),
  };
}

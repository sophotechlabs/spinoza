import dagre from '@dagrejs/dagre';
import type { Graph as DagreGraph, EdgeLabel, GraphLabel, NodeLabel } from '@dagrejs/dagre';
import type { Edge, Node } from '@xyflow/react';
import type {
  Graph,
  GraphEdge,
  GraphEdgeKind,
  GraphNode,
  GraphNodeCategory,
  ReadyState,
} from './types';

const NODE_WIDTH = 180;
const NODE_HEIGHT = 44;
const RANK_SEPARATION = 90;
const NODE_SEPARATION = 28;

const NODE_BASE_CLASS = 'rounded border px-2 py-1 text-[11px] font-mono';

export const EDGE_SOURCE_STROKE = 'var(--graph-edge-source)';
export const EDGE_DEPENDS_STROKE = 'var(--graph-edge-depends)';
export const EDGE_MANAGES_STROKE = 'var(--graph-edge-manages)';

interface GitopsNodeData {
  label: string;
  node: GraphNode;
  [key: string]: unknown;
}

export type GitopsFlowNode = Node<GitopsNodeData>;

export interface GitopsFlow {
  nodes: GitopsFlowNode[];
  edges: Edge[];
}

type LayoutGraph = DagreGraph<GraphLabel, NodeLabel, EdgeLabel>;

type StatusTone = 'ok' | 'error' | 'unknown';

export function statusTone(ready: ReadyState): StatusTone {
  if (ready === 'True') {
    return 'ok';
  }
  if (ready === 'False') {
    return 'error';
  }
  return 'unknown';
}

function nodeClassName(category: GraphNodeCategory, ready: ReadyState): string {
  const tone = statusTone(ready);
  if (tone === 'ok') {
    return `${NODE_BASE_CLASS} border-ok-emphasis bg-ok-tint text-ok-contrast`;
  }
  if (tone === 'error') {
    return `${NODE_BASE_CLASS} border-error-emphasis bg-error-tint text-error-contrast`;
  }
  if (category === 'source') {
    return `${NODE_BASE_CLASS} border-info-line bg-info-tint text-info-contrast`;
  }
  return `${NODE_BASE_CLASS} border-edge-emphasis bg-surface-active text-fg-strong`;
}

function edgeStroke(kind: GraphEdgeKind): string {
  if (kind === 'source') {
    return EDGE_SOURCE_STROKE;
  }
  if (kind === 'dependsOn') {
    return EDGE_DEPENDS_STROKE;
  }
  return EDGE_MANAGES_STROKE;
}

function edgeAnimated(kind: GraphEdgeKind): boolean {
  if (kind === 'source') {
    return true;
  }
  return false;
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

function toFlowNode(g: LayoutGraph, node: GraphNode): GitopsFlowNode {
  const laid = g.node(node.id);
  return {
    id: node.id,
    position: layoutPosition(laid),
    data: { label: node.name, node },
    className: nodeClassName(node.category, node.ready),
  };
}

function toFlowEdge(edge: GraphEdge): Edge {
  return {
    id: `${edge.from}->${edge.to}:${edge.kind}`,
    source: edge.from,
    target: edge.to,
    animated: edgeAnimated(edge.kind),
    style: { stroke: edgeStroke(edge.kind) },
    data: { kind: edge.kind },
  };
}

function sameNodeShape(a: GraphNode, b: GraphNode): boolean {
  if (a.id !== b.id) {
    return false;
  }
  if (a.name !== b.name) {
    return false;
  }
  return a.category === b.category;
}

function sameEdge(a: GraphEdge, b: GraphEdge): boolean {
  if (a.from !== b.from) {
    return false;
  }
  if (a.to !== b.to) {
    return false;
  }
  return a.kind === b.kind;
}

export function sameTopology(a: Graph, b: Graph): boolean {
  if (a.nodes.length !== b.nodes.length) {
    return false;
  }
  if (!a.nodes.every((node, index) => sameNodeShape(node, b.nodes[index]))) {
    return false;
  }
  if (a.edges.length !== b.edges.length) {
    return false;
  }
  return a.edges.every((edge, index) => sameEdge(edge, b.edges[index]));
}

export function sameGraph(a: Graph, b: Graph): boolean {
  if (!sameTopology(a, b)) {
    return false;
  }
  if (a.error !== b.error) {
    return false;
  }
  return a.nodes.every((node, index) => {
    const other = b.nodes[index];
    if (node.status !== other.status) {
      return false;
    }
    if (node.ready !== other.ready) {
      return false;
    }
    return node.namespace === other.namespace;
  });
}

export function restyle(flow: GitopsFlow, graph: Graph): GitopsFlow {
  const nodes = flow.nodes.map((node, index) => {
    const next = graph.nodes[index];
    return {
      ...node,
      data: { label: next.name, node: next },
      className: nodeClassName(next.category, next.ready),
    };
  });
  return { nodes, edges: flow.edges };
}

export function toFlow(graph: Graph): GitopsFlow {
  const g: LayoutGraph = new dagre.graphlib.Graph<GraphLabel, NodeLabel, EdgeLabel>();
  g.setGraph({ rankdir: 'LR', ranksep: RANK_SEPARATION, nodesep: NODE_SEPARATION });
  g.setDefaultEdgeLabel(() => ({}));
  for (const node of graph.nodes) {
    g.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  }
  for (const edge of graph.edges) {
    g.setEdge(edge.from, edge.to);
  }
  dagre.layout(g);
  const nodes = graph.nodes.map((node) => toFlowNode(g, node));
  const edges = graph.edges.map((edge) => toFlowEdge(edge));
  return { nodes, edges };
}

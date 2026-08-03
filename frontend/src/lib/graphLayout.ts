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

const EDGE_SOURCE_STROKE = '#0ea5e9';
const EDGE_DEPENDS_STROKE = '#f59e0b';
const EDGE_MANAGES_STROKE = '#525252';

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
    return `${NODE_BASE_CLASS} border-green-600 bg-green-950 text-green-200`;
  }
  if (tone === 'error') {
    return `${NODE_BASE_CLASS} border-red-600 bg-red-950 text-red-200`;
  }
  if (category === 'source') {
    return `${NODE_BASE_CLASS} border-sky-700 bg-sky-950 text-sky-200`;
  }
  return `${NODE_BASE_CLASS} border-neutral-600 bg-neutral-800 text-neutral-100`;
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

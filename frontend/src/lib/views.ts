import type { View } from './types';

export const VIEW_LABELS: Record<View, string> = {
  resources: 'Resources',
  cluster: 'Cluster Overview',
  issues: 'Issues',
  topology: 'Topology',
  helm: 'Helm releases',
  checks: 'Cluster checks',
  history: 'History',
  'flux-roles': 'Flux Overview',
  gitops: 'Flux Graph',
  'flux-list': 'Flux Resource list',
  'argo-apps': 'Argo CD Overview',
  'argo-graph': 'Argo CD Graph',
  'argo-list': 'Argo CD Resource list',
  traffic: 'Traffic',
  fleet: 'Fleet',
  rbac: 'Who can do what',
};

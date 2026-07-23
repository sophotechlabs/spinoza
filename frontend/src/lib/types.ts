export interface PodRow {
  uid: string;
  name: string;
  namespace: string;
  phase: string;
  ready: string;
  restarts: number;
  node: string;
  createdAt: string;
}

export type ServerMsg =
  | { type: 'snapshot'; resource: 'pods'; items: PodRow[]; rv: string }
  | { type: 'added' | 'modified'; resource: 'pods'; item: PodRow }
  | { type: 'deleted'; resource: 'pods'; uid: string }
  | { type: 'error'; message: string };

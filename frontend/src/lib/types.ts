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
}

export interface Row {
  uid: string;
  name: string;
  namespace: string;
  createdAt: string;
  cells: string[];
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

import { create } from 'zustand';
import type { Capability } from '../lib/access';
import type { ObjectRef } from '../lib/types';
import { refQuery } from '../lib/object';
import { useContextsStore } from './contexts';

export type Refusals = Partial<Record<Capability, string>>;

interface AccessState {
  // The cluster and object these answers are about. They are never shown for
  // anything else: a refusal can name the object it is about, and the same
  // object in another cluster is a different question entirely.
  key: string;
  refused: Refusals;
  setRefused: (key: string, refused: Refusals) => void;
  forget: () => void;
}

const NONE: Refusals = {};

export const useAccessStore = create<AccessState>((set) => ({
  key: '',
  refused: NONE,
  setRefused: (key, refused) => {
    set({ key, refused });
  },
  forget: () => {
    set({ key: '', refused: NONE });
  },
}));

// accessKey names the question: this object, in this cluster.
export function accessKey(context: string, ref: ObjectRef | null): string {
  if (ref === null) {
    return '';
  }
  return `${context}|${refQuery(ref)}`;
}

export function useRefusalsFor(ref: ObjectRef | null): Refusals {
  const context = useContextsStore((state) => state.list.current.name);
  const key = useAccessStore((state) => state.key);
  const refused = useAccessStore((state) => state.refused);
  if (ref === null) {
    return NONE;
  }
  if (key !== accessKey(context, ref)) {
    return NONE;
  }
  return refused;
}

// useRefusal is the reason this object's capability is out of reach, or null
// when nothing is known to stand in the way.
export function useRefusal(ref: ObjectRef | null, capability: Capability): string | null {
  return useRefusalsFor(ref)[capability] ?? null;
}

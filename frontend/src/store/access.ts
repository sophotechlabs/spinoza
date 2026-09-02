import { create } from 'zustand';
import type { Capability } from '../lib/access';
import type { ObjectRef } from '../lib/types';
import { refQuery } from '../lib/object';
import { useContextScope } from './contexts';

export type Refusals = Partial<Record<Capability, string>>;

interface AccessState {
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

export function accessKey(context: string, ref: ObjectRef | null): string {
  if (ref === null) {
    return '';
  }
  return `${context}|${refQuery(ref)}`;
}

export function useRefusalsFor(ref: ObjectRef | null): Refusals {
  const context = useContextScope();
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

export function useRefusal(ref: ObjectRef | null, capability: Capability): string | null {
  return useRefusalsFor(ref)[capability] ?? null;
}

import { create } from 'zustand';
import type { HelmCapability, HelmRefusals } from '../lib/helmAccess';
import { useContextsStore } from './contexts';

interface HelmAccessState {
  // A release panel and an install dialog can be open at once, so one answer at
  // a time would leave the first showing nothing.
  answers: Record<string, HelmRefusals>;
  setRefused: (key: string, refused: HelmRefusals) => void;
  forget: (key: string) => void;
}

const NONE: HelmRefusals = {};

function without(
  answers: Record<string, HelmRefusals>,
  dropped: string,
): Record<string, HelmRefusals> {
  const out: Record<string, HelmRefusals> = {};
  for (const [key, refusals] of Object.entries(answers)) {
    if (key === dropped) {
      continue;
    }
    out[key] = refusals;
  }
  return out;
}

export const useHelmAccessStore = create<HelmAccessState>((set) => ({
  answers: {},
  setRefused: (key, refused) => {
    set((state) => ({ answers: { ...state.answers, [key]: refused } }));
  },
  forget: (key) => {
    set((state) => ({ answers: without(state.answers, key) }));
  },
}));

export function helmAccessKey(context: string, namespace: string, name: string): string {
  if (namespace === '') {
    return '';
  }
  return `${context}|${namespace}|${name}`;
}

export function useHelmRefusals(namespace: string, name: string): HelmRefusals {
  const context = useContextsStore((state) => state.list.current.name);
  const key = helmAccessKey(context, namespace, name);
  const answers = useHelmAccessStore((state) => state.answers);
  return answers[key] ?? NONE;
}

export function useHelmRefusal(
  namespace: string,
  name: string,
  capability: HelmCapability,
): string | null {
  return useHelmRefusals(namespace, name)[capability] ?? null;
}

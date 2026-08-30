import { useState } from 'react';

export const MAX_NODES = 400;

export interface Laid<G, F> {
  graph: G;
  flow: F;
}

export interface Drawn<G, F> {
  laid: Laid<G, F> | null;
  partial: string | null;
  overLimit: number | null;
}

interface Drawable {
  nodes: unknown[];
  error?: string;
}

export function useLaidOut<G extends Drawable, F>(
  data: G | null,
  layOut: (current: Laid<G, F> | null, graph: G) => Laid<G, F>,
): Drawn<G, F> {
  const [laid, setLaid] = useState<Laid<G, F> | null>(null);

  let partial: string | null = null;
  let overLimit: number | null = null;
  if (data !== null) {
    partial = data.error ?? null;
    if (data.nodes.length > MAX_NODES) {
      overLimit = data.nodes.length;
    }
  }

  if (data === null && laid !== null) {
    setLaid(null);
  }

  if (data !== null && overLimit === null) {
    const next = layOut(laid, data);
    if (next !== laid) {
      setLaid(next);
    }
  }

  return { laid, partial, overLimit };
}

export const CLUSTER_COLORS = 8;

export function colorVar(color: number): string {
  return `var(--cluster-${String(held(color))})`;
}

export function held(color: number): number {
  if (color < 1) {
    return 1;
  }
  if (color > CLUSTER_COLORS) {
    return 1;
  }
  return color;
}

export function colorNames(): number[] {
  return Array.from({ length: CLUSTER_COLORS }, (_one, at) => at + 1);
}

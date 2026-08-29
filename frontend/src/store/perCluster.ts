export type ByCluster<T> = Partial<Record<string, T>>;

export function held<T>(map: ByCluster<T>, cluster: string, fallback: T): T {
  const found = map[cluster];
  if (found === undefined) {
    return fallback;
  }
  return found;
}

export function put<T>(map: ByCluster<T>, cluster: string, value: T): ByCluster<T> {
  return { ...map, [cluster]: value };
}

export function drop<T>(map: ByCluster<T>, cluster: string): ByCluster<T> {
  const next: ByCluster<T> = {};
  for (const [key, value] of Object.entries(map)) {
    if (key !== cluster) {
      next[key] = value;
    }
  }
  return next;
}

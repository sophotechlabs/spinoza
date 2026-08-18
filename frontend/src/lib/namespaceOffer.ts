export const PODS_WORTH_ASKING = 1000;

const PODS_KEY = '/v1/pods';

export function podsIn(counts: Partial<Record<string, number>>): number {
  const pods = counts[PODS_KEY];
  if (pods === undefined) {
    return 0;
  }
  return pods;
}

export function bigCluster(counts: Partial<Record<string, number>>): boolean {
  return podsIn(counts) >= PODS_WORTH_ASKING;
}

export function worthAsking(
  context: string,
  counts: Partial<Record<string, number>>,
  answered: boolean,
): boolean {
  if (context === '') {
    return false;
  }
  if (answered) {
    return false;
  }
  return bigCluster(counts);
}

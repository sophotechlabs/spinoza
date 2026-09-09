export function syncClass(sync: string): string {
  if (sync === 'Synced') {
    return 'text-ok';
  }
  if (sync === '') {
    return 'text-fg-muted';
  }
  return 'text-warn';
}

export function healthClass(health: string): string {
  if (health === 'Healthy') {
    return 'text-ok';
  }
  if (health === 'Degraded' || health === 'Missing') {
    return 'text-error';
  }
  if (health === '') {
    return 'text-fg-muted';
  }
  return 'text-warn';
}

export function orDash(value: string): string {
  if (value === '') {
    return '-';
  }
  return value;
}

export interface ArgoSummary {
  total: number;
  synced: number;
  healthy: number;
}

export function argoSummary(apps: { sync: string; health: string }[]): ArgoSummary {
  return {
    total: apps.length,
    synced: apps.filter((one) => one.sync === 'Synced').length,
    healthy: apps.filter((one) => one.health === 'Healthy').length,
  };
}

export function argoSummaryLabel(summary: ArgoSummary): string {
  if (summary.total === 0) {
    return 'No applications on this cluster';
  }
  return `${String(summary.total)} applications, ${String(summary.synced)} synced, ${String(summary.healthy)} healthy`;
}

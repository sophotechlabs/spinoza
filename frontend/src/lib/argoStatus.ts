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

export function ago(stamp: string, now: number): string {
  if (stamp === '') {
    return '';
  }
  const at = new Date(stamp).getTime();
  if (Number.isNaN(at)) {
    return '';
  }
  return since(Math.floor((now - at) / 1000));
}

export function since(seconds: number): string {
  let elapsed = seconds;
  if (elapsed < 0) {
    elapsed = 0;
  }
  if (elapsed < 60) {
    return `${String(elapsed)}s`;
  }
  const minutes = Math.floor(elapsed / 60);
  if (minutes < 60) {
    return `${String(minutes)}m`;
  }
  const hours = Math.floor(minutes / 60);
  if (hours < 24) {
    return `${String(hours)}h`;
  }
  const days = Math.floor(hours / 24);
  return `${String(days)}d`;
}

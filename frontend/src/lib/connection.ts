export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected';

export function offline(status: ConnectionStatus, attempt: number): boolean {
  if (status === 'disconnected') {
    return true;
  }
  if (status === 'connected') {
    return false;
  }
  return attempt > 0;
}

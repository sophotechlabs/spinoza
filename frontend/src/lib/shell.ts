import type { ShellState } from './types';

export function terminalTitle(shell: ShellState): string {
  if (shell === 'absent') {
    return 'No shell in this image — a debug container can be attached';
  }
  return 'Shell into the selected container';
}

import type { ShellState } from './types';

export function terminalTitle(shell: ShellState): string {
  if (shell === 'absent') {
    return 'No shell in this image, attach a debug container';
  }
  return 'Shell into the selected container';
}

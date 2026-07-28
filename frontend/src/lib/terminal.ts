import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

interface TerminalSize {
  cols: number;
  rows: number;
}

export interface TerminalHandle {
  write: (text: string) => void;
  onData: (handler: (data: string) => void) => void;
  fit: () => TerminalSize;
  focus: () => void;
  dispose: () => void;
}

export function createTerminal(node: HTMLElement): TerminalHandle {
  const term = new Terminal({
    convertEol: true,
    fontSize: 12,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
    cursorBlink: true,
    theme: { background: '#0a0a0a', foreground: '#d4d4d4' },
  });
  const fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(node);

  return {
    write: (text: string) => {
      term.write(text);
    },
    onData: (handler: (data: string) => void) => {
      term.onData(handler);
    },
    fit: () => {
      fitAddon.fit();
      return { cols: term.cols, rows: term.rows };
    },
    focus: () => {
      term.focus();
    },
    dispose: () => {
      term.dispose();
    },
  };
}

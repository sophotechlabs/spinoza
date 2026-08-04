import { Terminal } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import '@xterm/xterm/css/xterm.css';

interface TerminalSize {
  cols: number;
  rows: number;
}

export interface TerminalTheme {
  background: string;
  foreground: string;
}

const DEFAULT_TERMINAL_THEME: TerminalTheme = {
  background: '#0a0a0a',
  foreground: '#d4d4d4',
};

export interface TerminalHandle {
  write: (text: string) => void;
  onData: (handler: (data: string) => void) => void;
  setTheme: (theme: TerminalTheme) => void;
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
  });
  const fitAddon = new FitAddon();
  term.loadAddon(fitAddon);
  term.open(node);

  const handle: TerminalHandle = {
    write: (text: string) => {
      term.write(text);
    },
    setTheme: (theme: TerminalTheme) => {
      term.options.theme = theme;
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
  handle.setTheme(DEFAULT_TERMINAL_THEME);
  return handle;
}

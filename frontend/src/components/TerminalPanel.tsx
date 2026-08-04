import { useEffect, useRef, useState } from 'react';
import type { ExecTarget } from '../lib/types';
import { openExec } from '../lib/exec';
import { createTerminal } from '../lib/terminal';
import type { ExecEnd, ExecSession } from '../lib/exec';
import type { TerminalHandle } from '../lib/terminal';
import { terminalTheme } from '../lib/themeColors';
import { useResolvedTheme, useThemeStore } from '../store/theme';

interface TerminalPanelProps {
  target: ExecTarget;
  onShellMissing: () => void;
}

function endNotice(message: string): string {
  if (message === '') {
    return '\r\n\x1b[38;5;244msession ended\x1b[0m\r\n';
  }
  return `\r\n\x1b[38;5;214m${message}\x1b[0m\r\n`;
}

export default function TerminalPanel({ target, onShellMissing }: TerminalPanelProps) {
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const [ended, setEnded] = useState<ExecEnd | null>(null);
  const [attempt, setAttempt] = useState(0);
  const shellMissingRef = useRef(onShellMissing);
  shellMissingRef.current = onShellMissing;
  const termRef = useRef<TerminalHandle | null>(null);
  const resolvedTheme = useResolvedTheme();

  const { namespace, pod, container } = target;

  useEffect(() => {
    if (host === null) {
      return;
    }
    setEnded(null);

    const term: TerminalHandle = createTerminal(host);
    termRef.current = term;
    term.setTheme(terminalTheme(useThemeStore.getState().resolved));
    const session: ExecSession = openExec(
      { namespace, pod, container },
      {
        onOutput: (text) => {
          term.write(text);
        },
        onEnd: (end) => {
          term.write(endNotice(end.message));
          setEnded(end);
          if (end.message.includes('/bin/sh')) {
            shellMissingRef.current();
          }
        },
      },
    );

    term.onData((data) => {
      session.send(data);
    });

    function resize() {
      const size = term.fit();
      session.resize(size.cols, size.rows);
    }

    resize();
    term.focus();

    const observer = new ResizeObserver(resize);
    observer.observe(host);

    return () => {
      observer.disconnect();
      session.close();
      term.dispose();
      termRef.current = null;
    };
  }, [host, namespace, pod, container, attempt]);

  useEffect(() => {
    termRef.current?.setTheme(terminalTheme(resolvedTheme));
  }, [host, resolvedTheme]);

  function retry() {
    setAttempt((value) => value + 1);
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col border-t border-edge bg-surface">
      <div ref={setHost} className="min-h-0 flex-1" data-testid="terminal-host" />
      {ended !== null && ended.failed && (
        <div
          role="status"
          className="flex items-center gap-2 border-t border-edge px-2 py-1 text-[11px]"
        >
          <span className="break-words text-error">{ended.message}</span>
          <button
            type="button"
            onClick={retry}
            className="ml-auto shrink-0 rounded border border-edge-strong px-1.5 py-0.5 text-fg hover:bg-surface-active"
          >
            Retry
          </button>
        </div>
      )}
    </div>
  );
}

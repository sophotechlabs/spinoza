import { useEffect, useState } from 'react';
import type { ExecTarget } from '../lib/types';
import { openExec } from '../lib/exec';
import { createTerminal } from '../lib/terminal';
import type { ExecSession } from '../lib/exec';
import type { TerminalHandle } from '../lib/terminal';

interface TerminalPanelProps {
  target: ExecTarget;
  onShellMissing: () => void;
}

function endNotice(message: string): string {
  if (message === '') {
    return '\r\n\x1b[38;5;244msession ended\x1b[0m\r\n';
  }
  return `\r\n\x1b[38;5;203m${message}\x1b[0m\r\n`;
}

export default function TerminalPanel({ target, onShellMissing }: TerminalPanelProps) {
  const [host, setHost] = useState<HTMLDivElement | null>(null);
  const [ended, setEnded] = useState('');

  const { namespace, pod, container } = target;

  useEffect(() => {
    if (host === null) {
      return;
    }
    setEnded('');

    const term: TerminalHandle = createTerminal(host);
    const session: ExecSession = openExec(
      { namespace, pod, container },
      {
        onOutput: (text) => {
          term.write(text);
        },
        onEnd: (message) => {
          term.write(endNotice(message));
          setEnded(message);
          if (message.includes('/bin/sh')) {
            onShellMissing();
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
    };
  }, [host, namespace, pod, container, onShellMissing]);

  return (
    <div className="flex h-56 flex-col border-t border-neutral-800 bg-neutral-950">
      <div ref={setHost} className="min-h-0 flex-1" data-testid="terminal-host" />
      {ended !== '' && (
        <div className="border-t border-neutral-800 px-2 py-1 text-[11px] text-amber-400">
          {ended}
        </div>
      )}
    </div>
  );
}

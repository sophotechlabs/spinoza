import { Suspense, lazy, useCallback, useState } from 'react';
import { openExec, openLocalShell, openNodeShell } from '../lib/exec';
import type { ExecHandlers } from '../lib/exec';
import { useShellSupport } from '../lib/useShellSupport';
import DebugPrompt from './DebugPrompt';
import Loading from './Loading';

const TerminalPanel = lazy(() => import('./TerminalPanel'));

interface TerminalSessionProps {
  namespace: string;
  pod: string;
  container: string;
}

export function NodeTerminalSession({ node }: { node: string }) {
  const open = useCallback((handlers: ExecHandlers) => openNodeShell(node, handlers), [node]);
  const missing = useCallback(() => undefined, []);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <Suspense fallback={<Loading what="terminal" />}>
        <TerminalPanel key={`node/${node}`} openSession={open} onShellMissing={missing} />
      </Suspense>
    </div>
  );
}

export function LocalTerminalSession() {
  const open = useCallback((handlers: ExecHandlers) => openLocalShell(handlers), []);
  const missing = useCallback(() => undefined, []);
  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <Suspense fallback={<Loading what="terminal" />}>
        <TerminalPanel openSession={open} onShellMissing={missing} />
      </Suspense>
    </div>
  );
}

export default function TerminalSession({ namespace, pod, container }: TerminalSessionProps) {
  const [debugContainer, setDebugContainer] = useState<string | null>(null);
  const { shell, error: probeError, markMissing } = useShellSupport(namespace, pod, container);

  let target = container;
  if (debugContainer !== null) {
    target = debugContainer;
  }

  let needsDebugContainer = false;
  if (shell === 'absent' && debugContainer === null) {
    needsDebugContainer = true;
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      {probeError !== null && (
        <p role="status" className="shrink-0 border-b border-edge px-3 py-1 text-[11px] text-warn">
          Could not check {container} for a shell: {probeError}. Opening one anyway.
        </p>
      )}
      {needsDebugContainer && (
        <DebugPrompt
          key={`${namespace}/${pod}/${container}`}
          target={{ namespace, pod, container }}
          onAttached={setDebugContainer}
        />
      )}
      {!needsDebugContainer && (
        <Suspense fallback={<Loading what="terminal" />}>
          <TerminalPanel
            key={`${namespace}/${pod}/${target}`}
            openSession={(handlers) => openExec({ namespace, pod, container: target }, handlers)}
            onShellMissing={markMissing}
          />
        </Suspense>
      )}
    </div>
  );
}

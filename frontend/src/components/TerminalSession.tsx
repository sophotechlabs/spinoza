import { Suspense, lazy, useState } from 'react';
import { useShellSupport } from '../lib/useShellSupport';
import DebugPrompt from './DebugPrompt';
import Loading from './Loading';

const TerminalPanel = lazy(() => import('./TerminalPanel'));

interface TerminalSessionProps {
  namespace: string;
  pod: string;
  container: string;
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
          Could not check whether {container} has a shell: {probeError}. Opening a session anyway.
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
            target={{ namespace, pod, container: target }}
            onShellMissing={markMissing}
          />
        </Suspense>
      )}
    </div>
  );
}

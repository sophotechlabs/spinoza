import { useState } from 'react';
import type { PodTarget } from '../lib/pods';
import { firstContainer } from '../lib/pods';
import { useShellSupport } from '../lib/useShellSupport';
import DebugPrompt from './DebugPrompt';
import TerminalPanel from './TerminalPanel';

interface TerminalTabProps {
  pod: PodTarget | null;
}

export default function TerminalTab({ pod }: TerminalTabProps) {
  const [container, setContainer] = useState(() => firstContainer(pod));
  const [debugContainer, setDebugContainer] = useState<string | null>(null);

  const podKey = pod === null ? '' : `${pod.namespace}/${pod.name}`;
  const podNamespace = pod === null ? '' : pod.namespace;
  const podName = pod === null ? '' : pod.name;

  const [lastPodKey, setLastPodKey] = useState(podKey);
  if (podKey !== lastPodKey) {
    setLastPodKey(podKey);
    setContainer(firstContainer(pod));
    setDebugContainer(null);
  }

  const { shell, markMissing } = useShellSupport(podNamespace, podName, container);

  let terminalContainer = container;
  if (debugContainer !== null) {
    terminalContainer = debugContainer;
  }

  let needsDebugContainer = false;
  if (shell === 'absent' && debugContainer === null) {
    needsDebugContainer = true;
  }

  if (pod === null || container === '') {
    return (
      <div className="p-3 text-[11px] text-neutral-600">Select a pod to open a shell in it.</div>
    );
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center gap-2 border-b border-neutral-900 px-3 py-1.5 text-xs">
        <span className="truncate text-neutral-500">
          {pod.namespace}/{pod.name}
        </span>
        {pod.containers.length > 1 && (
          <select
            aria-label="Container"
            value={container}
            onChange={(event) => {
              setContainer(event.target.value);
              setDebugContainer(null);
            }}
            className="rounded border border-neutral-700 bg-neutral-900 px-1 py-0.5 text-neutral-200"
          >
            {pod.containers.map((name) => (
              <option key={name} value={name}>
                {name}
              </option>
            ))}
          </select>
        )}
      </div>
      {needsDebugContainer && (
        <DebugPrompt
          target={{ namespace: podNamespace, pod: podName, container }}
          onAttached={setDebugContainer}
        />
      )}
      {!needsDebugContainer && (
        <TerminalPanel
          key={`${podNamespace}/${podName}/${terminalContainer}`}
          target={{ namespace: podNamespace, pod: podName, container: terminalContainer }}
          onShellMissing={markMissing}
        />
      )}
    </div>
  );
}

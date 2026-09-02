import { useEffect, useState } from 'react';
import type { LogRequest } from './types';
import { useClusterEpoch } from '../store/cluster';

export const TAIL_LINES = 500;

let streamSeq = 0;

function nextLogSubId(prefix: string): string {
  streamSeq += 1;
  return `${prefix}#${String(streamSeq)}`;
}

interface LogStream {
  prefix: string;
  namespace: string;
  name: string;
  container: string;
  workload?: { group: string; version: string; resource: string };
  enabled: boolean;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

export function useLogStream(stream: LogStream): string {
  const { prefix, namespace, name, container, workload, enabled, subscribeLogs, unsubscribeLogs } =
    stream;
  const [subId, setSubId] = useState('');
  const target = workloadKey(workload);
  const epoch = useClusterEpoch();
  const [lastEpoch, setLastEpoch] = useState(epoch);

  if (epoch !== lastEpoch) {
    setLastEpoch(epoch);
    setSubId('');
  }

  useEffect(() => {
    if (!enabled) {
      setSubId('');
      return;
    }
    if (container === '' && target === '') {
      setSubId('');
      return;
    }
    const id = nextLogSubId(prefix);
    setSubId(id);
    subscribeLogs(id, {
      namespace,
      name,
      container,
      tailLines: TAIL_LINES,
      follow: true,
      ...workloadFrom(target),
    });
    return () => {
      unsubscribeLogs(id);
    };
  }, [prefix, namespace, name, container, target, enabled, subscribeLogs, unsubscribeLogs, epoch]);

  return subId;
}

function workloadKey(workload: LogStream['workload']): string {
  if (workload === undefined) {
    return '';
  }
  return `${workload.group}/${workload.version}/${workload.resource}`;
}

function workloadFrom(key: string): Partial<LogRequest> {
  if (key === '') {
    return {};
  }
  const [group, version, resource] = key.split('/');
  return { group, version, resource };
}

import { useEffect, useState } from 'react';
import type { LogRequest } from './types';

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
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

export function useLogStream(stream: LogStream): string {
  const { prefix, namespace, name, container, subscribeLogs, unsubscribeLogs } = stream;
  const [subId, setSubId] = useState('');

  useEffect(() => {
    if (container === '') {
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
    });
    return () => {
      unsubscribeLogs(id);
    };
  }, [prefix, namespace, name, container, subscribeLogs, unsubscribeLogs]);

  return subId;
}

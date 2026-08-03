import { useEffect } from 'react';
import type { LogRequest } from './types';

export const TAIL_LINES = 500;

interface LogStream {
  subId: string;
  namespace: string;
  name: string;
  container: string;
  subscribeLogs: (subId: string, request: LogRequest) => void;
  unsubscribeLogs: (subId: string) => void;
}

export function useLogStream(stream: LogStream) {
  const { subId, namespace, name, container, subscribeLogs, unsubscribeLogs } = stream;

  useEffect(() => {
    if (container === '') {
      return;
    }
    subscribeLogs(subId, {
      namespace,
      name,
      container,
      tailLines: TAIL_LINES,
      follow: true,
    });
    return () => {
      unsubscribeLogs(subId);
    };
  }, [subId, namespace, name, container, subscribeLogs, unsubscribeLogs]);
}

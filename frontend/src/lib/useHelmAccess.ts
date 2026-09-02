import { useEffect } from 'react';
import { fetchHelmAccess, helmRefusalsOf } from './helmAccess';
import { helmAccessKey, useHelmAccessStore } from '../store/helmAccess';
import { useContextScope } from '../store/contexts';

export function useHelmAccess(namespace: string, name: string): void {
  const context = useContextScope();
  const key = helmAccessKey(context, namespace, name);

  useEffect(() => {
    const { setRefused, forget } = useHelmAccessStore.getState();
    if (key === '') {
      return;
    }
    let mounted = true;
    const load = async () => {
      try {
        const access = await fetchHelmAccess(namespace, name);
        if (mounted) {
          setRefused(key, helmRefusalsOf(access));
        }
      } catch {
        if (mounted) {
          forget(key);
        }
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [key, namespace, name]);
}

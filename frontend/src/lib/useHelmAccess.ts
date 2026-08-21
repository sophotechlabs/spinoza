import { useEffect } from 'react';
import { fetchHelmAccess, helmRefusalsOf } from './helmAccess';
import { helmAccessKey, useHelmAccessStore } from '../store/helmAccess';
import { useContextsStore } from '../store/contexts';

// useHelmAccess asks the cluster what it would refuse a helm action in this
// namespace, once per release. A question that cannot be answered leaves every
// button alone, and switching cluster or namespace asks again.
export function useHelmAccess(namespace: string, name: string): void {
  const context = useContextsStore((state) => state.list.current.name);
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

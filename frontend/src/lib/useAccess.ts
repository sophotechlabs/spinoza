import { useEffect } from 'react';
import type { ObjectRef } from './types';
import { accessQuery, fetchAccess, refusalsOf } from './access';
import { useAccessStore } from '../store/access';

// useAccess asks the cluster what it would refuse for the selected object, once
// per selection. A question that cannot be answered leaves every button alone.
export function useAccess(target: ObjectRef | null): void {
  const key = target === null ? '' : accessQuery(target);

  useEffect(() => {
    const { setRefused, forget } = useAccessStore.getState();
    if (key === '') {
      forget();
      return;
    }
    let mounted = true;
    const load = async () => {
      try {
        const access = await fetchAccess(key);
        if (mounted) {
          setRefused(key, refusalsOf(access));
        }
      } catch {
        if (mounted) {
          forget();
        }
      }
    };
    void load();
    return () => {
      mounted = false;
    };
  }, [key]);
}

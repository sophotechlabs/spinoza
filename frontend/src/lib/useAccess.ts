import { useEffect } from 'react';
import type { ObjectRef } from './types';
import { fetchAccess, refusalsOf } from './access';
import { refQuery } from './object';
import { accessKey, useAccessStore } from '../store/access';
import { useContextScope } from '../store/contexts';

export function useAccess(target: ObjectRef | null): void {
  const context = useContextScope();
  const key = accessKey(context, target);
  const query = target === null ? '' : refQuery(target);

  useEffect(() => {
    const { setRefused, forget } = useAccessStore.getState();
    if (key === '') {
      forget();
      return;
    }
    let mounted = true;
    const load = async () => {
      try {
        const access = await fetchAccess(query);
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
  }, [key, query]);
}

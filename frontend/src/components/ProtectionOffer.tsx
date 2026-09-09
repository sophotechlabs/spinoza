import { useEffect, useRef } from 'react';
import { setProtection } from '../lib/contexts';
import { useContextList, useContextScope, useContextsStore } from '../store/contexts';
import { askToast, notifyError, notifyOk } from '../store/toasts';

function reason(err: unknown): string {
  if (err instanceof Error && err.message !== '') {
    return err.message;
  }
  return 'the answer could not be saved';
}

export default function ProtectionOffer() {
  const list = useContextList();
  const clusterScope = useContextScope();
  const setList = useContextsStore((state) => state.setList);
  const asked = useRef(new Set<string>());
  const asking = list.protection === 'unknown' && list.current.name !== '';
  const name = list.current.name;

  useEffect(() => {
    if (!asking) {
      return;
    }
    if (asked.current.has(clusterScope)) {
      return;
    }
    asked.current.add(clusterScope);

    async function answer(wanted: boolean) {
      try {
        const found = await setProtection(wanted);
        setList(found);
        if (wanted) {
          notifyOk(`${name} is protected`);
        }
      } catch (err: unknown) {
        notifyError(reason(err));
      }
    }

    askToast(
      `${name} is new here. Protect it? Deleting, draining, scaling to zero and uninstalling would then need the object name typed first.`,
      [
        {
          label: 'Protect',
          run: () => {
            void answer(true);
          },
        },
        {
          label: 'Leave unprotected',
          run: () => {
            void answer(false);
          },
        },
      ],
    );
  }, [asking, clusterScope, name, setList]);

  return null;
}

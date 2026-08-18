import { useEffect } from 'react';
import type { RefObject } from 'react';

export function outside(menu: HTMLDetailsElement, target: EventTarget | null): boolean {
  if (!menu.open) {
    return false;
  }
  if (target instanceof Node) {
    return !menu.contains(target);
  }
  return true;
}

export function useDismissMenu(ref: RefObject<HTMLDetailsElement | null>): void {
  useEffect(() => {
    function close() {
      const menu = ref.current;
      if (menu !== null) {
        menu.open = false;
      }
    }

    function handlePointerDown(event: PointerEvent) {
      const menu = ref.current;
      if (menu !== null && outside(menu, event.target)) {
        close();
      }
    }

    function handleFocusIn(event: FocusEvent) {
      const menu = ref.current;
      if (menu !== null && outside(menu, event.target)) {
        close();
      }
    }

    function handleKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') {
        close();
      }
    }

    document.addEventListener('pointerdown', handlePointerDown);
    document.addEventListener('focusin', handleFocusIn);
    document.addEventListener('keydown', handleKeyDown);
    window.addEventListener('blur', close);
    return () => {
      document.removeEventListener('pointerdown', handlePointerDown);
      document.removeEventListener('focusin', handleFocusIn);
      document.removeEventListener('keydown', handleKeyDown);
      window.removeEventListener('blur', close);
    };
  }, [ref]);
}

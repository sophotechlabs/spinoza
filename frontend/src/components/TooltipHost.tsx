import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import type { TooltipPlacement, TooltipTarget } from '../lib/tooltip';
import {
  TOOLTIP_DELAY_MS,
  claimTitle,
  droppedTitle,
  place,
  releaseTitle,
  tooltipHost,
} from '../lib/tooltip';

export default function TooltipHost() {
  const [target, setTarget] = useState<TooltipTarget | null>(null);
  const [placement, setPlacement] = useState<TooltipPlacement | null>(null);
  const tipRef = useRef<HTMLDivElement | null>(null);
  const shown = useRef(false);
  const tipId = useId();

  useEffect(() => {
    let timer: ReturnType<typeof setTimeout> | null = null;
    let held: HTMLElement | null = null;
    let watcher: MutationObserver | null = null;
    let label = '';

    function cancel() {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
      if (watcher !== null) {
        watcher.disconnect();
        watcher = null;
      }
      releaseTitle(held);
      held = null;
      label = '';
      shown.current = false;
      setTarget(null);
      setPlacement(null);
    }

    function show(host: HTMLElement) {
      shown.current = true;
      setTarget({ label, rect: host.getBoundingClientRect() });
    }

    function watch(host: HTMLElement) {
      watcher = new MutationObserver(() => {
        if (droppedTitle(host)) {
          cancel();
          return;
        }
        const next = claimTitle(host, tipId);
        if (next === '' || next === label) {
          return;
        }
        label = next;
        if (shown.current) {
          show(host);
        }
      });
      watcher.observe(host, { attributes: true, attributeFilter: ['title'] });
    }

    function open(node: EventTarget | null, delay: number) {
      const host = tooltipHost(node);
      if (host === null || host === held) {
        return;
      }
      cancel();
      label = claimTitle(host, tipId);
      if (label === '') {
        return;
      }
      held = host;
      watch(host);
      timer = setTimeout(() => {
        show(host);
      }, delay);
    }

    function onOver(event: MouseEvent) {
      open(event.target, TOOLTIP_DELAY_MS);
    }

    function onFocus(event: FocusEvent) {
      open(event.target, 0);
    }

    document.addEventListener('mouseover', onOver);
    document.addEventListener('mouseout', cancel);
    document.addEventListener('focusin', onFocus);
    document.addEventListener('focusout', cancel);
    document.addEventListener('keydown', cancel);
    window.addEventListener('scroll', cancel, true);
    return () => {
      cancel();
      document.removeEventListener('mouseover', onOver);
      document.removeEventListener('mouseout', cancel);
      document.removeEventListener('focusin', onFocus);
      document.removeEventListener('focusout', cancel);
      document.removeEventListener('keydown', cancel);
      window.removeEventListener('scroll', cancel, true);
    };
  }, [tipId]);

  useLayoutEffect(() => {
    const tip = tipRef.current;
    if (target === null || tip === null) {
      return;
    }
    const viewport = new DOMRect(0, 0, window.innerWidth, window.innerHeight);
    setPlacement(place(target.rect, tip.getBoundingClientRect(), viewport));
  }, [target]);

  if (target === null) {
    return null;
  }

  let style: TooltipPlacement = { left: 0, top: 0 };
  let hidden = 'opacity-0';
  if (placement !== null) {
    style = placement;
    hidden = 'opacity-100';
  }

  return (
    <div
      ref={tipRef}
      id={tipId}
      role="tooltip"
      className={`pointer-events-none fixed z-50 max-w-80 rounded border border-edge-strong bg-surface-raised px-2 py-1 text-xs text-fg shadow-lg ${hidden}`}
      style={{ left: `${style.left}px`, top: `${style.top}px` }}
    >
      {target.label}
    </div>
  );
}

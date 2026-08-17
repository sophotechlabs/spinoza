export const TOOLTIP_DELAY_MS = 120;

const TOOLTIP_GAP = 6;

export const TOOLTIP_ATTRIBUTE = 'data-tooltip';

export interface TooltipPlacement {
  left: number;
  top: number;
}

export interface TooltipTarget {
  label: string;
  rect: DOMRect;
}

export function tooltipHost(node: EventTarget | null): HTMLElement | null {
  if (!(node instanceof Element)) {
    return null;
  }
  const found = node.closest('[title], [' + TOOLTIP_ATTRIBUTE + ']');
  if (!(found instanceof HTMLElement)) {
    return null;
  }
  return found;
}

export const HELD_TITLE = '';

export function heldTitle(host: HTMLElement): boolean {
  return host.getAttribute('title') === HELD_TITLE;
}

export function droppedTitle(host: HTMLElement): boolean {
  return host.getAttribute('title') === null;
}

export function claimTitle(host: HTMLElement, describedBy: string): string {
  const title = host.getAttribute('title');
  if (title === null || title === HELD_TITLE) {
    const kept = host.getAttribute(TOOLTIP_ATTRIBUTE);
    if (kept === null || kept === '') {
      return '';
    }
    host.setAttribute('aria-describedby', describedBy);
    return kept;
  }
  host.setAttribute(TOOLTIP_ATTRIBUTE, title);
  host.setAttribute('aria-describedby', describedBy);
  host.setAttribute('title', HELD_TITLE);
  return title;
}

export function releaseTitle(host: HTMLElement | null): void {
  if (host === null) {
    return;
  }
  const title = host.getAttribute(TOOLTIP_ATTRIBUTE);
  if (title === null) {
    return;
  }
  if (heldTitle(host)) {
    host.setAttribute('title', title);
  }
  host.removeAttribute(TOOLTIP_ATTRIBUTE);
  host.removeAttribute('aria-describedby');
}

export function place(anchor: DOMRect, tip: DOMRect, viewport: DOMRect): TooltipPlacement {
  let left = anchor.left + anchor.width / 2 - tip.width / 2;
  const rightEdge = viewport.width - TOOLTIP_GAP - tip.width;
  if (left > rightEdge) {
    left = rightEdge;
  }
  if (left < TOOLTIP_GAP) {
    left = TOOLTIP_GAP;
  }
  let top = anchor.bottom + TOOLTIP_GAP;
  if (top + tip.height > viewport.height - TOOLTIP_GAP) {
    top = anchor.top - TOOLTIP_GAP - tip.height;
  }
  if (top < TOOLTIP_GAP) {
    top = TOOLTIP_GAP;
  }
  return { left, top };
}

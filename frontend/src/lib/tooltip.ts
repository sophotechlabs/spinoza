export const TOOLTIP_DELAY_MS = 120;

export const TOOLTIP_GAP = 6;

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

export function claimTitle(host: HTMLElement, describedBy: string): string {
  const title = host.getAttribute('title') ?? host.getAttribute(TOOLTIP_ATTRIBUTE);
  if (title === null || title === '') {
    return '';
  }
  host.setAttribute(TOOLTIP_ATTRIBUTE, title);
  host.setAttribute('aria-describedby', describedBy);
  host.removeAttribute('title');
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
  host.setAttribute('title', title);
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

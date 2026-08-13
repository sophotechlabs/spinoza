import { describe, expect, it } from 'vitest';
import {
  TOOLTIP_ATTRIBUTE,
  claimTitle,
  place,
  releaseTitle,
  tooltipHost,
} from '../../src/lib/tooltip';

function element(html: string): HTMLElement {
  document.body.innerHTML = html;
  const found = document.body.firstElementChild;
  if (!(found instanceof HTMLElement)) {
    throw new Error('no element');
  }
  return found;
}

describe('tooltipHost', () => {
  it('finds the titled element the pointer is over', () => {
    const host = element('<button title="Reconnect">go</button>');

    expect(tooltipHost(host)).toBe(host);
  });

  it('climbs to the titled ancestor of the thing actually hovered', () => {
    const host = element('<span title="Latest"><em>text</em></span>');
    const inner = host.querySelector('em');

    expect(tooltipHost(inner)).toBe(host);
  });

  it('has nothing for an element with no title anywhere above it', () => {
    const host = element('<span>plain</span>');

    expect(tooltipHost(host)).toBeNull();
  });

  it('has nothing for a target that is not an element', () => {
    expect(tooltipHost(null)).toBeNull();
    expect(tooltipHost(document)).toBeNull();
  });
});

describe('claimTitle', () => {
  it('takes the title off the element so the slow native tooltip cannot fire', () => {
    const host = element('<button title="Reconnect">go</button>');

    expect(claimTitle(host, 'tip-1')).toBe('Reconnect');
    expect(host.hasAttribute('title')).toBe(false);
    expect(host.getAttribute(TOOLTIP_ATTRIBUTE)).toBe('Reconnect');
  });

  it('points a screen reader at the tooltip that replaced the title', () => {
    const host = element('<button title="Reconnect">go</button>');

    claimTitle(host, 'tip-1');

    expect(host).toHaveAttribute('aria-describedby', 'tip-1');
  });

  it('reports nothing for an empty title', () => {
    const host = element('<button title="">go</button>');

    expect(claimTitle(host, 'tip-1')).toBe('');
    expect(host.hasAttribute('aria-describedby')).toBe(false);
  });

  it('still speaks for an element left claimed by an earlier hover', () => {
    const host = element(`<button ${TOOLTIP_ATTRIBUTE}="Reconnect">go</button>`);

    expect(tooltipHost(host)).toBe(host);
    expect(claimTitle(host, 'tip-2')).toBe('Reconnect');
    expect(host).toHaveAttribute('aria-describedby', 'tip-2');
  });
});

describe('releaseTitle', () => {
  it('puts the title back when the tooltip closes', () => {
    const host = element('<button title="Reconnect">go</button>');
    claimTitle(host, 'tip-1');

    releaseTitle(host);

    expect(host).toHaveAttribute('title', 'Reconnect');
    expect(host.hasAttribute(TOOLTIP_ATTRIBUTE)).toBe(false);
    expect(host.hasAttribute('aria-describedby')).toBe(false);
  });

  it('leaves an element it never claimed alone', () => {
    const host = element('<button>go</button>');

    releaseTitle(host);

    expect(host.hasAttribute('title')).toBe(false);
  });

  it('does nothing when there is no element', () => {
    expect(() => {
      releaseTitle(null);
    }).not.toThrow();
  });
});

describe('place', () => {
  const viewport = new DOMRect(0, 0, 1000, 800);

  it('centres under the thing it describes', () => {
    const anchor = new DOMRect(400, 100, 100, 20);
    const tip = new DOMRect(0, 0, 200, 30);

    expect(place(anchor, tip, viewport)).toEqual({ left: 350, top: 126 });
  });

  it('stays inside the right edge', () => {
    const anchor = new DOMRect(960, 100, 40, 20);
    const tip = new DOMRect(0, 0, 200, 30);

    expect(place(anchor, tip, viewport).left).toBe(794);
  });

  it('stays inside the left edge', () => {
    const anchor = new DOMRect(0, 100, 20, 20);
    const tip = new DOMRect(0, 0, 200, 30);

    expect(place(anchor, tip, viewport).left).toBe(6);
  });

  it('flips above the anchor when there is no room below', () => {
    const anchor = new DOMRect(400, 760, 100, 20);
    const tip = new DOMRect(0, 0, 200, 30);

    expect(place(anchor, tip, viewport).top).toBe(724);
  });

  it('gives up and sits at the top when neither side fits', () => {
    const anchor = new DOMRect(400, 0, 100, 780);
    const tip = new DOMRect(0, 0, 200, 30);

    expect(place(anchor, tip, viewport).top).toBe(6);
  });
});

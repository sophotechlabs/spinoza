import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import TooltipHost from '../../src/components/TooltipHost';
import { TOOLTIP_DELAY_MS } from '../../src/lib/tooltip';

function withTitle(title: string): HTMLElement {
  const host = document.createElement('button');
  host.textContent = 'target';
  host.setAttribute('title', title);
  document.body.append(host);
  return host;
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  document.body.innerHTML = '';
});

function settle(ms: number): void {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
}

describe('TooltipHost', () => {
  it('shows nothing until something is hovered', () => {
    render(<TooltipHost />);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('opens shortly after the pointer arrives, not a second later', () => {
    render(<TooltipHost />);
    const host = withTitle('The latest revision the source offers for this resource');

    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS - 1);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
    settle(1);

    expect(screen.getByRole('tooltip')).toHaveTextContent(
      'The latest revision the source offers for this resource',
    );
  });

  it('takes the title off the element so the browser does not draw its own', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');

    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    expect(host.hasAttribute('title')).toBe(false);
    expect(host).toHaveAttribute('data-tooltip', 'Reconnect');
    expect(host).toHaveAttribute('aria-describedby', screen.getByRole('tooltip').id);
  });

  it('gives the title back when the pointer leaves', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');
    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    fireEvent.mouseOut(host);

    expect(host).toHaveAttribute('title', 'Reconnect');
    expect(host.hasAttribute('aria-describedby')).toBe(false);
  });

  it('reads the text of a child that carries no title of its own', () => {
    render(<TooltipHost />);
    const host = withTitle('Latest');
    const inner = document.createElement('span');
    host.append(inner);

    fireEvent.mouseOver(inner);
    settle(TOOLTIP_DELAY_MS);

    expect(screen.getByRole('tooltip')).toHaveTextContent('Latest');
  });

  it('closes when the pointer leaves', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');
    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    fireEvent.mouseOut(host);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('never opens if the pointer moves on before the delay is up', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');

    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS - 20);
    fireEvent.mouseOut(host);
    settle(TOOLTIP_DELAY_MS);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('opens at once for the keyboard, which has no hover', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');

    fireEvent.focusIn(host);
    settle(0);

    expect(screen.getByRole('tooltip')).toBeInTheDocument();
  });

  it('closes on blur and on a keypress', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');
    fireEvent.focusIn(host);
    settle(0);
    fireEvent.focusOut(host);
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();

    fireEvent.focusIn(host);
    settle(0);
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('closes when the page scrolls out from under it', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');
    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    fireEvent.scroll(window);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('ignores an element with nothing to say', () => {
    render(<TooltipHost />);
    const plain = document.createElement('button');
    document.body.append(plain);

    fireEvent.mouseOver(plain);
    settle(TOOLTIP_DELAY_MS);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('ignores an element whose title is empty', () => {
    render(<TooltipHost />);
    const host = withTitle('');

    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });

  it('swaps to whatever is hovered next', () => {
    render(<TooltipHost />);
    const first = withTitle('first');
    const second = withTitle('second');
    fireEvent.mouseOver(first);
    settle(TOOLTIP_DELAY_MS);

    fireEvent.mouseOver(second);
    settle(TOOLTIP_DELAY_MS);

    expect(screen.getByRole('tooltip')).toHaveTextContent('second');
  });

  it('is positioned once it has been measured', () => {
    render(<TooltipHost />);
    const host = withTitle('Reconnect');

    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    const tip = screen.getByRole('tooltip');
    expect(tip.style.left).not.toBe('');
    expect(tip.style.top).not.toBe('');
  });

  it('stops listening once it is gone', () => {
    const view = render(<TooltipHost />);
    const host = withTitle('Reconnect');

    view.unmount();
    fireEvent.mouseOver(host);
    settle(TOOLTIP_DELAY_MS);

    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument();
  });
});

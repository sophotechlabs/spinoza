import { describe, expect, it } from 'vitest';
import { useRef } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { outside, useDismissMenu } from '../../src/lib/useDismissMenu';

function Menu() {
  const ref = useRef<HTMLDetailsElement | null>(null);
  useDismissMenu(ref);
  return (
    <div>
      <details ref={ref} open data-testid="menu">
        <summary>Cluster</summary>
        <button type="button">p-mk1</button>
      </details>
      <button type="button">elsewhere</button>
    </div>
  );
}

function menu(): HTMLDetailsElement {
  return screen.getByTestId('menu');
}

// which targets count as being outside the menu

function detailsWith(open: boolean): HTMLDetailsElement {
  const element = document.createElement('details');
  element.open = open;
  const child = document.createElement('button');
  element.append(child);
  return element;
}

describe('what counts as a click outside', () => {
  it('says no while the menu is already closed', () => {
    expect(outside(detailsWith(false), document.body)).toBe(false);
  });

  it('says no for the menu itself and what it holds', () => {
    const element = detailsWith(true);

    expect(outside(element, element)).toBe(false);
    expect(outside(element, element.firstChild)).toBe(false);
  });

  it('says yes for another element', () => {
    expect(outside(detailsWith(true), document.createElement('div'))).toBe(true);
  });

  it('says yes when the event names no node at all', () => {
    expect(outside(detailsWith(true), null)).toBe(true);
  });
});

// what closes an open menu

describe('dismissing an open menu', () => {
  it('closes when something else is clicked', () => {
    render(<Menu />);

    fireEvent.pointerDown(screen.getByRole('button', { name: 'elsewhere' }));

    expect(menu().open).toBe(false);
  });

  it('stays open when the click is inside it', () => {
    render(<Menu />);

    fireEvent.pointerDown(screen.getByRole('button', { name: 'p-mk1' }));

    expect(menu().open).toBe(true);
  });

  it('closes when focus moves out of it', () => {
    render(<Menu />);

    fireEvent.focusIn(screen.getByRole('button', { name: 'elsewhere' }));

    expect(menu().open).toBe(false);
  });

  it('stays open when focus moves within it', () => {
    render(<Menu />);

    fireEvent.focusIn(screen.getByRole('button', { name: 'p-mk1' }));

    expect(menu().open).toBe(true);
  });

  it('closes on escape', () => {
    render(<Menu />);

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(menu().open).toBe(false);
  });

  it('ignores other keys', () => {
    render(<Menu />);

    fireEvent.keyDown(document, { key: 'a' });

    expect(menu().open).toBe(true);
  });

  it('closes when the window loses focus', () => {
    render(<Menu />);

    fireEvent.blur(window);

    expect(menu().open).toBe(false);
  });

  it('stops listening once it is gone', () => {
    const view = render(<Menu />);
    const element = menu();

    view.unmount();
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(element.open).toBe(true);
  });
});

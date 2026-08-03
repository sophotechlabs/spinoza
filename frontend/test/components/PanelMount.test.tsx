import { describe, expect, it } from 'vitest';
import { useCallback, useState } from 'react';
import { act, render, screen } from '@testing-library/react';
import PanelMount from '../../src/components/PanelMount';

function Counter() {
  const [count, setCount] = useState(0);
  return (
    <button
      type="button"
      onClick={() => {
        setCount((value) => value + 1);
      }}
    >
      count {count}
    </button>
  );
}

function Harness({ hostId, active }: { hostId: 'a' | 'b' | 'none'; active: boolean }) {
  const [hosts, setHosts] = useState<Record<string, HTMLDivElement | null>>({ a: null, b: null });
  const setA = useCallback((node: HTMLDivElement | null) => {
    setHosts((current) => (current.a === node ? current : { ...current, a: node }));
  }, []);
  const setB = useCallback((node: HTMLDivElement | null) => {
    setHosts((current) => (current.b === node ? current : { ...current, b: node }));
  }, []);
  let host: HTMLDivElement | null = null;
  if (hostId !== 'none') {
    host = hosts[hostId];
  }
  return (
    <div>
      <div data-testid="host-a" ref={setA} />
      <div data-testid="host-b" ref={setB} />
      <PanelMount host={host} active={active}>
        <Counter />
      </PanelMount>
    </div>
  );
}

function parentOf(node: HTMLElement): HTMLElement {
  const parent = node.parentElement;
  if (parent === null) {
    throw new Error('expected the element to have a parent');
  }
  return parent;
}

describe('PanelMount', () => {
  it('renders its panel inside the host it was given', () => {
    render(<Harness hostId="a" active />);

    expect(screen.getByTestId('host-a')).toContainElement(screen.getByRole('button'));
  });

  it('hides the panel when it is not the open one', () => {
    render(<Harness hostId="a" active={false} />);

    const hidden = screen.getByRole('button', { hidden: true });
    expect(parentOf(hidden).style.display).toBe('none');
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('shows the panel when it becomes the open one', () => {
    const view = render(<Harness hostId="a" active={false} />);

    view.rerender(<Harness hostId="a" active />);

    const wrapper = parentOf(screen.getByRole('button'));
    expect(wrapper.style.display).toBe('flex');
  });

  it('keeps panel state when it is moved to another host', () => {
    const view = render(<Harness hostId="a" active />);
    act(() => {
      screen.getByRole('button').click();
    });
    expect(screen.getByRole('button')).toHaveTextContent('count 1');

    view.rerender(<Harness hostId="b" active />);

    expect(screen.getByTestId('host-b')).toContainElement(screen.getByRole('button'));
    expect(screen.getByRole('button')).toHaveTextContent('count 1');
  });

  it('holds the panel out of the page until a host exists', () => {
    render(<Harness hostId="none" active />);

    expect(screen.getByTestId('host-a')).toBeEmptyDOMElement();
    expect(screen.getByTestId('host-b')).toBeEmptyDOMElement();
  });
});

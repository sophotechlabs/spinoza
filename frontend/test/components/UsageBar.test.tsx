import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import UsageBar from '../../src/components/UsageBar';

function bar(container: HTMLElement): HTMLElement {
  const el = container.querySelector('span > span > span');
  if (el === null) {
    throw new Error('bar element not found');
  }
  return el as HTMLElement;
}

describe('UsageBar', () => {
  it('shows the percent label, a title and a green bar for low usage', () => {
    const { container } = render(<UsageBar percent={20} label="200m" />);
    expect(screen.getByText('20%')).toBeInTheDocument();
    expect(screen.getByTitle('200m')).toBeInTheDocument();
    const fill = bar(container);
    expect(fill.className).toContain('bg-ok-solid');
    expect(fill.style.width).toBe('20%');
  });

  it('clamps over-100 percent to full width and colors it red', () => {
    const { container } = render(<UsageBar percent={130} label="hot" />);
    const fill = bar(container);
    expect(fill.className).toContain('bg-error-solid');
    expect(fill.style.width).toBe('100%');
  });

  it('clamps negative percent to zero width', () => {
    const { container } = render(<UsageBar percent={-5} label="x" />);
    expect(bar(container).style.width).toBe('0%');
  });
});

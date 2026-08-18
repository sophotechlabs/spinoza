import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import Loading from '../../src/components/Loading';

describe('Loading', () => {
  it('says what it is waiting for', () => {
    render(<Loading what="pods" />);

    expect(screen.getByRole('status')).toHaveTextContent('Loading pods');
  });

  it('turns while it waits, and holds still when motion is not wanted', () => {
    const { container } = render(<Loading what="pods" />);
    const spinner = container.querySelector('svg');

    expect(spinner?.getAttribute('class')).toContain('animate-spin');
    expect(spinner?.getAttribute('class')).toContain('motion-reduce:animate-none');
    expect(spinner).toHaveAttribute('aria-hidden', 'true');
  });
});

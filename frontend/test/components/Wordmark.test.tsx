import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import Wordmark from '../../src/components/Wordmark';

describe('Wordmark', () => {
  it('spells the name in capitals, tracked out, in the display face', () => {
    render(<Wordmark />);

    const mark = screen.getByText('SPINOZA');
    expect(mark).toHaveClass('font-display');
    expect(mark).toHaveClass('font-medium');
    expect(mark).toHaveClass('tracking-[0.18em]');
  });

  it('takes the colour it is given', () => {
    render(<Wordmark className="text-fg-muted" />);

    expect(screen.getByText('SPINOZA')).toHaveClass('text-fg-muted');
  });
});

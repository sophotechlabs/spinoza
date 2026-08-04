import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import Announce from '../../src/components/Announce';

describe('Announce', () => {
  it('stays in the document with nothing to say, so the region exists before the news', () => {
    const { container } = render(<Announce message={null} />);

    expect(container.querySelector('[role="status"]')).not.toBeNull();
    expect(screen.getByRole('status')).toBeEmptyDOMElement();
  });

  it('carries no styling while it is empty', () => {
    render(<Announce message={null} className="mt-3 text-error" />);

    expect(screen.getByRole('status').className).toBe('');
  });

  it('says a polite message', () => {
    render(<Announce message="Applied." className="text-ok" />);

    const region = screen.getByRole('status');
    expect(region).toHaveTextContent('Applied.');
    expect(region.className).toBe('text-ok');
  });

  it('interrupts for an urgent one', () => {
    render(<Announce message="apply failed" urgent />);

    expect(screen.getByRole('alert')).toHaveTextContent('apply failed');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('../src/App', () => ({
  default: () => {
    throw new Error('App exploded');
  },
}));

import Root from '../src/Root';

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => undefined);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe('the boundary around the whole app', () => {
  it('catches what App itself throws, so the window is never left blank', () => {
    render(<Root />);

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('Spinoza stopped rendering');
    expect(alert).toHaveTextContent('App exploded');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });
});

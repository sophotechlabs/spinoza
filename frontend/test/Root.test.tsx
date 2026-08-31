import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('../src/App', () => ({
  default: () => {
    throw new Error('App exploded');
  },
}));

import Root from '../src/Root';
import { OWN_WINDOW } from '../src/lib/identity';

function answers(body: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve({ ok, status: 200, json: () => Promise.resolve(body) })),
  );
}

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => undefined);
});

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe('the boundary around the whole app', () => {
  it('catches what App itself throws, so the window is never left blank', async () => {
    answers(OWN_WINDOW);

    render(<Root />);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Spinoza stopped rendering');
    expect(alert).toHaveTextContent('App exploded');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });

  it('waits for the backend to say who you are before it renders anything', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    );

    render(<Root />);

    expect(screen.getByRole('status')).toHaveTextContent('Loading spinoza');
  });

  it('asks a served spinoza to sign you in instead of rendering the app', async () => {
    answers({ authenticated: false, cluster: true, mode: 'oidc', role: '', signIn: true });

    render(<Root />);

    expect(await screen.findByTestId('sign-in')).toHaveAttribute('href', '/auth/login?next=%2F');
  });

  it('falls back to your own window when the session request throws', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.reject(new Error('offline'))),
    );

    render(<Root />);

    expect(await screen.findByRole('alert')).toHaveTextContent('App exploded');
  });
});

import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import SignIn from '../../src/components/SignIn';
import { OWN_WINDOW } from '../../src/lib/identity';
import type { Session } from '../../src/lib/types';

function served(overrides: Partial<Session>): Session {
  return { ...OWN_WINDOW, authenticated: false, cluster: true, role: '', ...overrides };
}

describe('the sign-in page', () => {
  it('offers the button when the backend can start a login', () => {
    render(<SignIn session={served({ signIn: true, mode: 'oidc' })} />);

    expect(screen.getByTestId('sign-in')).toBeInTheDocument();
    expect(screen.getByText(/identity provider to reach this cluster/)).toBeInTheDocument();
  });

  it('says a proxy was meant to name you when there is no login to offer', () => {
    render(<SignIn session={served({ signIn: false, mode: 'proxy' })} />);

    expect(screen.queryByTestId('sign-in')).not.toBeInTheDocument();
    expect(screen.getByText(/no identity reached it/)).toBeInTheDocument();
  });

  it('shows what went wrong when a login came back refused', () => {
    render(<SignIn session={served({ signIn: true, error: 'the id token did not verify' })} />);

    expect(screen.getByText('the id token did not verify')).toBeInTheDocument();
  });
});

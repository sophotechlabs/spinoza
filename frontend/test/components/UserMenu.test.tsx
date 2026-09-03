import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import UserMenu from '../../src/components/UserMenu';
import { OWN_WINDOW } from '../../src/lib/identity';
import { adoptSession } from '../../src/store/identity';
import type { Session } from '../../src/lib/types';

function signedIn(overrides: Partial<Session>): Session {
  return {
    ...OWN_WINDOW,
    authenticated: true,
    cluster: true,
    mode: 'oidc',
    role: 'editor',
    user: 'alice@example.com',
    ...overrides,
  };
}

describe('the account menu', () => {
  it('stays away in a window you started yourself', () => {
    adoptSession(OWN_WINDOW);

    const { container } = render(<UserMenu />);

    expect(container).toBeEmptyDOMElement();
  });

  it('stays away until somebody has signed in', () => {
    adoptSession({ ...OWN_WINDOW, cluster: true, authenticated: false, role: '' });

    const { container } = render(<UserMenu />);

    expect(container).toBeEmptyDOMElement();
  });

  it('stays away when nothing asks people to sign in', () => {
    adoptSession(signedIn({ mode: 'none', user: undefined }));

    const { container } = render(<UserMenu />);

    expect(container).toBeEmptyDOMElement();
  });

  it('names who is signed in, their role and what they can read', () => {
    adoptSession(signedIn({ groups: ['platform', 'sre'] }));

    render(<UserMenu />);

    expect(screen.getByLabelText('Account')).toHaveTextContent('alice@example.com');
    expect(screen.getByText(/Role editor, reading every namespace/)).toBeInTheDocument();
    expect(screen.getByText('platform, sre')).toBeInTheDocument();
    expect(screen.getByTestId('sign-out').tagName).toBe('FORM');
    expect(screen.getByTestId('sign-out')).toHaveAttribute('action', '/auth/logout');
    expect(screen.getByTestId('sign-out')).toHaveAttribute('method', 'post');
    expect(screen.getByRole('button', { name: 'Sign out' })).toHaveAttribute('type', 'submit');
  });

  it('says which namespaces a scoped account reads', () => {
    adoptSession(
      signedIn({
        role: 'viewer',
        scope: { everywhere: false, namespaces: ['payments'], undecided: [] },
      }),
    );

    render(<UserMenu />);

    expect(screen.getByText(/reading the payments namespace/)).toBeInTheDocument();
  });

  it('falls back to a plain label when the backend named no user', () => {
    adoptSession(signedIn({ user: undefined, groups: [] }));

    render(<UserMenu />);

    expect(screen.getByLabelText('Account')).toHaveTextContent('signed in');
  });
});

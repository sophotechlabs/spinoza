import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import LoadWarning from '../../src/components/LoadWarning';
import { WARNING_LIMIT, shortened } from '../../src/lib/warningText';

const short = 'secrets could not be listed cluster-wide';

const namespaces = [
  'argocd',
  'cert-manager',
  'default',
  'flux-system',
  'ingress-nginx',
  'kube-system',
  'monitoring',
  'observability',
  'production',
  'staging',
];

const long = `secrets could not be listed cluster-wide; 10 of 44 namespaces allowed it: ${namespaces.join(', ')}`;

describe('LoadWarning', () => {
  it('shows a short message whole, with nothing to expand', () => {
    render(<LoadWarning message={short} />);

    expect(screen.getByRole('status')).toHaveTextContent(short);
    expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument();
  });

  it('cuts a long message down and offers the rest', () => {
    render(<LoadWarning message={long} />);

    expect(screen.getByRole('status').textContent).not.toContain('staging');
    expect(screen.getByRole('button', { name: 'Show more' })).toBeInTheDocument();
  });

  it('shows every namespace once the rest is asked for', async () => {
    const user = userEvent.setup();
    render(<LoadWarning message={long} />);

    await user.click(screen.getByRole('button', { name: 'Show more' }));

    expect(screen.getByRole('status')).toHaveTextContent(long);
    expect(screen.getByRole('button', { name: 'Show less' })).toBeInTheDocument();
  });

  it('folds the message back up again', async () => {
    const user = userEvent.setup();
    render(<LoadWarning message={long} />);

    await user.click(screen.getByRole('button', { name: 'Show more' }));
    await user.click(screen.getByRole('button', { name: 'Show less' }));

    expect(screen.getByRole('status').textContent).not.toContain('staging');
  });

  it('cuts on a word boundary', () => {
    const cut = shortened(long);

    expect(long.startsWith(cut.slice(0, -1))).toBe(true);
    expect(cut.endsWith('…')).toBe(true);
    expect(cut).not.toContain(', …');
  });

  it('cuts mid-word when the message has no spaces to cut on', () => {
    const solid = 'x'.repeat(WARNING_LIMIT + 20);

    expect(shortened(solid)).toBe(`${'x'.repeat(WARNING_LIMIT)}…`);
  });

  it('leaves a message that already fits alone', () => {
    expect(shortened(short)).toBe(short);
  });
});

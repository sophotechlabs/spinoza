import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import DetailsDrawer from '../../src/components/DetailsDrawer';
import { makeRow } from '../helpers';

describe('DetailsDrawer', () => {
  it('shows the placeholder when no row is selected', () => {
    render(<DetailsDrawer row={null} />);
    expect(screen.getByText('Select a row to see details.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
  });

  it('renders every row field when a row is selected', () => {
    const row = makeRow({
      uid: 'uid-9',
      name: 'web-1',
      namespace: 'prod',
      createdAt: '2026-07-01T00:00:00Z',
    });
    const { container } = render(<DetailsDrawer row={row} />);
    const list = container.querySelector('dl');
    if (!list) {
      throw new Error('field list not found');
    }
    const fields = within(list);
    for (const label of ['Name', 'Namespace', 'Created', 'UID']) {
      expect(fields.getByText(label)).toBeInTheDocument();
    }
    expect(fields.getByText('web-1')).toBeInTheDocument();
    expect(fields.getByText('prod')).toBeInTheDocument();
    expect(fields.getByText('2026-07-01T00:00:00Z')).toBeInTheDocument();
    expect(fields.getByText('uid-9')).toBeInTheDocument();
  });

  it('calls onClose when the close button is clicked', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<DetailsDrawer row={makeRow({ uid: 'a' })} onClose={onClose} />);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not throw when close is clicked without an onClose handler', async () => {
    const user = userEvent.setup();
    render(<DetailsDrawer row={makeRow({ uid: 'a' })} />);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument();
  });
});

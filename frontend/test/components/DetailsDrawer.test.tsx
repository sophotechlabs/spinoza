import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import DetailsDrawer from '../../src/components/DetailsDrawer';
import { makePod } from '../helpers';

describe('DetailsDrawer', () => {
  it('shows the placeholder when no pod is selected', () => {
    render(<DetailsDrawer pod={null} />);
    expect(screen.getByText('Select a pod to see details.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
  });

  it('renders every pod field when a pod is selected', () => {
    const pod = makePod({
      uid: 'uid-9',
      name: 'web-1',
      namespace: 'prod',
      phase: 'Running',
      ready: '2/2',
      restarts: 3,
      node: 'node-b',
      createdAt: '2026-07-01T00:00:00Z',
    });
    const { container } = render(<DetailsDrawer pod={pod} />);
    const list = container.querySelector('dl');
    if (!list) {
      throw new Error('field list not found');
    }
    const fields = within(list);
    const labels = ['Name', 'Namespace', 'Status', 'Ready', 'Restarts', 'Node', 'Created', 'UID'];
    for (const label of labels) {
      expect(fields.getByText(label)).toBeInTheDocument();
    }
    expect(fields.getByText('web-1')).toBeInTheDocument();
    expect(fields.getByText('prod')).toBeInTheDocument();
    expect(fields.getByText('Running')).toBeInTheDocument();
    expect(fields.getByText('2/2')).toBeInTheDocument();
    expect(fields.getByText('3')).toBeInTheDocument();
    expect(fields.getByText('node-b')).toBeInTheDocument();
    expect(fields.getByText('2026-07-01T00:00:00Z')).toBeInTheDocument();
    expect(fields.getByText('uid-9')).toBeInTheDocument();
  });

  it('calls onClose when the close button is clicked', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<DetailsDrawer pod={makePod({ uid: 'a' })} onClose={onClose} />);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not throw when close is clicked without an onClose handler', async () => {
    const user = userEvent.setup();
    render(<DetailsDrawer pod={makePod({ uid: 'a' })} />);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument();
  });
});

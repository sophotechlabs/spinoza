import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import GitopsNodePanel from '../../src/components/GitopsNodePanel';
import { makeGraphNode } from '../helpers';

describe('GitopsNodePanel', () => {
  it('shows the placeholder when no node is selected', () => {
    render(<GitopsNodePanel node={null} />);
    expect(screen.getByText('Select a node to see details.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Close' })).not.toBeInTheDocument();
  });

  it('renders every node field when a node is selected', () => {
    const node = makeGraphNode({
      id: 'node-9',
      kind: 'Kustomization',
      group: 'kustomize.toolkit.fluxcd.io',
      name: 'apps',
      namespace: 'flux-system',
      status: 'Ready',
      category: 'applier',
    });
    const { container } = render(<GitopsNodePanel node={node} />);
    const list = container.querySelector('dl');
    if (!list) {
      throw new Error('field list not found');
    }
    const fields = within(list);
    for (const label of ['Kind', 'Group', 'Namespace', 'Status', 'Category', 'ID']) {
      expect(fields.getByText(label)).toBeInTheDocument();
    }
    expect(fields.getByText('Kustomization')).toBeInTheDocument();
    expect(fields.getByText('kustomize.toolkit.fluxcd.io')).toBeInTheDocument();
    expect(fields.getByText('Ready')).toBeInTheDocument();
    expect(fields.getByText('applier')).toBeInTheDocument();
    expect(fields.getByText('node-9')).toBeInTheDocument();
    expect(screen.getByText('apps')).toBeInTheDocument();
  });

  it('calls onClose when the close button is clicked', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<GitopsNodePanel node={makeGraphNode({ id: 'a' })} onClose={onClose} />);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('does not throw when close is clicked without an onClose handler', async () => {
    const user = userEvent.setup();
    render(<GitopsNodePanel node={makeGraphNode({ id: 'a' })} />);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.getByRole('button', { name: 'Close' })).toBeInTheDocument();
  });
});

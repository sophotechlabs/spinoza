import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import App from '../src/App';
import { usePodStore } from '../src/store/pods';
import { makePod } from './helpers';

vi.mock('../src/lib/feed', () => ({
  usePodsFeed: () => ({ status: 'connected' }),
}));

function resetStore(): void {
  usePodStore.setState({ rows: new Map(), sorted: [] });
}

describe('App', () => {
  beforeEach(() => {
    resetStore();
    usePodStore
      .getState()
      .applySnapshot([
        makePod({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
        makePod({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
      ]);
  });

  afterEach(() => {
    resetStore();
  });

  it('renders the feed status, the pod table and the empty details placeholder', () => {
    render(<App />);
    expect(screen.getByText('connected')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'pod-a' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'pod-b' })).toBeInTheDocument();
    expect(screen.getByText('Select a pod to see details.')).toBeInTheDocument();
  });

  it('opens and closes the details drawer when a pod is selected', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: 'pod-a' }));
    const drawer = screen.getByRole('complementary');
    expect(within(drawer).getByText('Namespace')).toBeInTheDocument();
    expect(within(drawer).getAllByText('pod-a')).toHaveLength(2);
    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.getByText('Select a pod to see details.')).toBeInTheDocument();
  });

  it('toggles the bottom dock panel', async () => {
    const user = userEvent.setup();
    render(<App />);
    await user.click(screen.getByRole('button', { name: /Panel/ }));
    expect(screen.getByText('No output.')).toBeInTheDocument();
  });
});

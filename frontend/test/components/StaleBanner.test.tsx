import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import StaleBanner from '../../src/components/StaleBanner';
import { useFeedStore } from '../../src/store/feed';

describe('StaleBanner', () => {
  it('names what stopped updating and why, as a live region', () => {
    render(
      <StaleBanner what="Metrics" message="metrics-server is down" onRetry={() => undefined} />,
    );

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('Metrics stopped updating.');
    expect(banner).toHaveTextContent('metrics-server is down');
  });

  it('asks for a fresh load when Retry is pressed', async () => {
    const onRetry = vi.fn();
    render(<StaleBanner what="Events" message="down" onRetry={onRetry} />);

    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });
});

describe('a view banner while the shell already explains the silence', () => {
  it('stays out of the way when the cluster feed dropped', () => {
    useFeedStore.getState().report('disconnected', 1);

    render(<StaleBanner what="The issue queue" message="Failed to fetch" onRetry={vi.fn()} />);

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('says its piece while everything else is fine', () => {
    render(<StaleBanner what="The issue queue" message="boom" onRetry={vi.fn()} />);

    expect(screen.getByRole('status')).toHaveTextContent('The issue queue stopped updating');
  });

  it('offers no retry when the caller has nothing to retry', () => {
    render(<StaleBanner what="Pod" message="the watch broke" />);

    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
  });
});

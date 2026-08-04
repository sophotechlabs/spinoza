import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import StaleBanner from '../../src/components/StaleBanner';

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

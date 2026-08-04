import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ConnectionBanner from '../../src/components/ConnectionBanner';
import { offline } from '../../src/lib/feed';

describe('offline', () => {
  it('is true once the socket has dropped', () => {
    expect(offline('disconnected', 0)).toBe(true);
  });

  it('is false while connected', () => {
    expect(offline('connected', 0)).toBe(false);
    expect(offline('connected', 4)).toBe(false);
  });

  it('stays quiet on the very first connect', () => {
    expect(offline('connecting', 0)).toBe(false);
  });

  it('is true while a retry is in flight', () => {
    expect(offline('connecting', 2)).toBe(true);
  });
});

describe('ConnectionBanner', () => {
  it('shows nothing while the feed is healthy', () => {
    const { container } = render(
      <ConnectionBanner status="connected" attempt={0} onReconnect={vi.fn()} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('shows nothing on the very first connect', () => {
    const { container } = render(
      <ConnectionBanner status="connecting" attempt={0} onReconnect={vi.fn()} />,
    );

    expect(container).toBeEmptyDOMElement();
  });

  it('says the data on screen is the last thing the cluster sent', () => {
    render(<ConnectionBanner status="disconnected" attempt={0} onReconnect={vi.fn()} />);

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('The live connection dropped');
    expect(banner).toHaveTextContent('Reconnecting…');
  });

  it('counts the retries out loud', () => {
    render(<ConnectionBanner status="connecting" attempt={3} onReconnect={vi.fn()} />);

    expect(screen.getByRole('status')).toHaveTextContent('Reconnecting — attempt 3.');
  });

  it('reconnects on demand', async () => {
    const onReconnect = vi.fn();
    render(<ConnectionBanner status="disconnected" attempt={1} onReconnect={onReconnect} />);

    await userEvent.click(screen.getByRole('button', { name: 'Reconnect now' }));

    expect(onReconnect).toHaveBeenCalledTimes(1);
  });
});

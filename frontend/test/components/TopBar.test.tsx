import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TopBar from '../../src/components/TopBar';

function dotFor(container: HTMLElement): Element {
  const dot = container.querySelector('span.rounded-full');
  if (!dot) {
    throw new Error('status dot not found');
  }
  return dot;
}

describe('TopBar', () => {
  it('shows a green dot when connected', () => {
    const { container } = render(<TopBar status="connected" />);
    expect(screen.getByText('connected')).toBeInTheDocument();
    expect(dotFor(container).className).toContain('bg-green-500');
  });

  it('shows a yellow dot when connecting', () => {
    const { container } = render(<TopBar status="connecting" />);
    expect(screen.getByText('connecting')).toBeInTheDocument();
    expect(dotFor(container).className).toContain('bg-yellow-500');
  });

  it('shows a red dot when disconnected', () => {
    const { container } = render(<TopBar status="disconnected" />);
    expect(screen.getByText('disconnected')).toBeInTheDocument();
    expect(dotFor(container).className).toContain('bg-red-500');
  });

  it('calls onReconnect when the reconnect button is clicked', async () => {
    const user = userEvent.setup();
    const onReconnect = vi.fn();
    render(<TopBar status="disconnected" onReconnect={onReconnect} />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(onReconnect).toHaveBeenCalledTimes(1);
  });

  it('does not throw when reconnect is clicked without a handler', async () => {
    const user = userEvent.setup();
    render(<TopBar status="disconnected" />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
  });
});

import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CapabilityState from '../../src/components/CapabilityState';
import { WARNING_LIMIT, shortened } from '../../src/lib/warningText';
import { useFeedStore } from '../../src/store/feed';

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

describe('waiting for something', () => {
  it('says what it is waiting for', () => {
    render(<CapabilityState state="loading" what="pods" />);

    expect(screen.getByRole('status')).toHaveTextContent('Loading pods');
  });

  it('turns while it waits, and holds still when motion is not wanted', () => {
    const { container } = render(<CapabilityState state="loading" what="pods" />);
    const spinner = container.querySelector('svg');

    expect(spinner?.getAttribute('class')).toContain('animate-spin');
    expect(spinner?.getAttribute('class')).toContain('motion-reduce:animate-none');
    expect(spinner).toHaveAttribute('aria-hidden', 'true');
  });
});

describe('nothing to show', () => {
  it('says so in the view its own words', () => {
    render(<CapabilityState state="empty" why="This cluster has no Pod objects." />);

    expect(screen.getByText('This cluster has no Pod objects.')).toBeVisible();
  });

  it('makes room for the way out a view offers', () => {
    render(
      <CapabilityState state="empty" why="Nothing matches the current filter.">
        <button type="button">Clear filter</button>
      </CapabilityState>,
    );

    expect(screen.getByRole('button', { name: 'Clear filter' })).toBeVisible();
  });
});

describe('a partial answer', () => {
  it('shows a short message whole, with nothing to expand', () => {
    render(<CapabilityState state="partial" why={short} />);

    expect(screen.getByRole('status')).toHaveTextContent(short);
    expect(screen.queryByRole('button', { name: 'Show more' })).not.toBeInTheDocument();
  });

  it('cuts a long message down and offers the rest', () => {
    render(<CapabilityState state="partial" why={long} />);

    expect(screen.getByRole('status').textContent).not.toContain('staging');
    expect(screen.getByRole('button', { name: 'Show more' })).toBeInTheDocument();
  });

  it('shows every namespace once the rest is asked for', async () => {
    const user = userEvent.setup();
    render(<CapabilityState state="partial" why={long} />);

    await user.click(screen.getByRole('button', { name: 'Show more' }));

    expect(screen.getByRole('status')).toHaveTextContent(long);
    expect(screen.getByRole('button', { name: 'Show less' })).toBeInTheDocument();
  });

  it('folds the message back up again', async () => {
    const user = userEvent.setup();
    render(<CapabilityState state="partial" why={long} />);

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

describe('data that stopped moving', () => {
  it('names what stopped updating and why, as a live region', () => {
    render(
      <CapabilityState
        state="stale"
        what="Metrics"
        why="metrics-server is down"
        onRetry={() => undefined}
      />,
    );

    const banner = screen.getByRole('status');
    expect(banner).toHaveTextContent('Metrics stopped updating.');
    expect(banner).toHaveTextContent('metrics-server is down');
  });

  it('asks for a fresh load when Retry is pressed', async () => {
    const onRetry = vi.fn();
    render(<CapabilityState state="stale" what="Events" why="down" onRetry={onRetry} />);

    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));

    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('offers no retry when the caller has nothing to retry', () => {
    render(<CapabilityState state="stale" what="Pod" why="the watch broke" />);

    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
  });
});

describe('something that could not be read at all', () => {
  it('names it, says why, and offers a way to try again', async () => {
    const onRetry = vi.fn();
    render(
      <CapabilityState
        state="failed"
        what="The cluster overview"
        why="the cluster did not answer"
        onRetry={onRetry}
      />,
    );

    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent('The cluster overview could not be loaded');
    expect(alert).toHaveTextContent('the cluster did not answer');

    await userEvent.click(screen.getByRole('button', { name: 'Retry' }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('leaves the retry out when the caller offers none', () => {
    render(<CapabilityState state="failed" what="Pod" why="boom" />);

    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull();
  });
});

describe('while the shell already explains the silence', () => {
  it('keeps a partial-data note out of the way', () => {
    useFeedStore.getState().report('disconnected', 1);

    render(<CapabilityState state="partial" why="3 of 31 resource types could not be listed" />);

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('keeps a stopped-updating note out of the way', () => {
    useFeedStore.getState().report('disconnected', 1);

    render(<CapabilityState state="stale" what="The issue queue" why="Failed to fetch" />);

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('still shows a failure, because nothing else is saying it', () => {
    useFeedStore.getState().report('disconnected', 1);

    render(<CapabilityState state="failed" what="The fleet" why="boom" />);

    expect(screen.getByRole('alert')).toBeVisible();
  });
});

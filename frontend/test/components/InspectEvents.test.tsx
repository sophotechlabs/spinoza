import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import InspectEvents from '../../src/components/InspectEvents';
import type { K8sEvent } from '../../src/lib/types';

function event(overrides: Partial<K8sEvent> = {}): K8sEvent {
  return {
    type: 'Normal',
    reason: 'Pulled',
    message: 'image pulled',
    source: 'kubelet',
    count: 1,
    firstSeen: '2026-07-27T09:00:00Z',
    lastSeen: '2026-07-27T09:30:00Z',
    ...overrides,
  };
}

function stubEvents(events: K8sEvent[]): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(events) }),
  );
}

describe('InspectEvents', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows a loading state first', () => {
    stubEvents([]);
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    expect(screen.getByText('Loading events…')).toBeInTheDocument();
  });

  it('reports when there are no events', async () => {
    stubEvents([]);
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    expect(await screen.findByText('No events for this object.')).toBeInTheDocument();
  });

  it('renders events with their source and timestamp', async () => {
    stubEvents([event(), event({ reason: 'BackOff', type: 'Warning', count: 5 })]);
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    expect(await screen.findByText('Pulled')).toBeInTheDocument();
    expect(screen.getByText('BackOff')).toHaveClass('text-amber-400');
    expect(screen.getByText('Pulled')).toHaveClass('text-neutral-400');
    expect(screen.getByText('seen 5 times')).toBeInTheDocument();
    expect(screen.getAllByText('2026-07-27T09:30:00Z')).toHaveLength(2);
  });

  it('surfaces a request failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'events blew up' }),
      }),
    );
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    expect(await screen.findByText('events blew up')).toBeInTheDocument();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    expect(await screen.findByText('events request failed')).toBeInTheDocument();
  });
});

describe('InspectEvents polling', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('refetches on an interval so the list does not go stale', async () => {
    vi.useFakeTimers();
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) });
    vi.stubGlobal('fetch', mock);
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    expect(mock).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(10000);

    expect(mock).toHaveBeenCalledTimes(2);
  });

  it('stops polling once unmounted', async () => {
    vi.useFakeTimers();
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) });
    vi.stubGlobal('fetch', mock);
    const view = render(<InspectEvents namespace="flux-system" uid="pod-uid" />);

    view.unmount();
    await vi.advanceTimersByTimeAsync(30000);

    expect(mock).toHaveBeenCalledTimes(1);
  });
});

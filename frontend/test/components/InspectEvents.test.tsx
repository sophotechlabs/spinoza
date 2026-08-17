import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
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

    expect(screen.getByText('Loading events')).toBeInTheDocument();
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
    expect(screen.getByText('BackOff')).toHaveClass('text-warn');
    expect(screen.getByText('Pulled')).toHaveClass('text-fg-muted');
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

  it('keeps the events it has and says they stopped updating', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve([event()]) });
        }
        return Promise.reject(new Error('events endpoint is down'));
      }),
    );
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('Pulled')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('events endpoint is down');
    expect(screen.getByText('Pulled')).toBeInTheDocument();
  });

  it('says an empty list stopped updating too', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
        }
        return Promise.reject(new Error('events endpoint is down'));
      }),
    );
    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('events endpoint is down');
    expect(screen.getByText('No events for this object.')).toBeInTheDocument();
  });
});

describe('a slow events poll', () => {
  it('does not stack a second request on top of the first', async () => {
    vi.useFakeTimers();
    let resolveFirst: (value: unknown) => void = () => undefined;
    const fetchMock = vi.fn(
      () =>
        new Promise((resolve) => {
          resolveFirst = resolve;
        }),
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<InspectEvents namespace="flux-system" uid="pod-uid" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveFirst({ ok: true, json: () => Promise.resolve([]) });
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    vi.useRealTimers();
  });
});

describe('an events panel behind another tab', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('does not poll at all while it is hidden', async () => {
    vi.useFakeTimers();
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) });
    vi.stubGlobal('fetch', mock);

    render(<InspectEvents namespace="flux-system" uid="pod-uid" active={false} />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(60000);
    });

    expect(mock).not.toHaveBeenCalled();
  });

  it('picks the poll back up when its tab comes forward', async () => {
    vi.useFakeTimers();
    const mock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve([]) });
    vi.stubGlobal('fetch', mock);
    const view = render(<InspectEvents namespace="flux-system" uid="pod-uid" active={false} />);

    view.rerender(<InspectEvents namespace="flux-system" uid="pod-uid" active />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(mock).toHaveBeenCalledTimes(1);
  });
});

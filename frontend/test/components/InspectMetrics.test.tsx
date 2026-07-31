import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectMetrics from '../../src/components/InspectMetrics';
import type { ChartHandle } from '../../src/lib/chart';
import type { MetricPoint } from '../../src/lib/types';

const createChart = vi.fn<(node: HTMLElement, options: unknown) => ChartHandle>();
const updates: MetricPoint[][] = [];
const resizes: number[] = [];

vi.mock('../../src/lib/chart', () => ({
  createChart: (node: HTMLElement, options: unknown) => createChart(node, options),
}));

function chartStub(): ChartHandle {
  return {
    update: (points: MetricPoint[]) => {
      updates.push(points);
    },
    resize: (width: number) => {
      resizes.push(width);
    },
    destroy: vi.fn(),
  };
}

function history(overrides: Record<string, unknown> = {}) {
  return {
    namespace: 'monitoring',
    pod: 'loki-0',
    source: 'monitoring/prometheus-operated:9090 (https)',
    cpu: [
      { at: 100, value: 0.02 },
      { at: 160, value: 0.05 },
    ],
    memory: [
      { at: 100, value: 390721536 },
      { at: 160, value: 400000000 },
    ],
    ...overrides,
  };
}

function stub(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  createChart.mockReset();
  createChart.mockImplementation(() => chartStub());
  updates.length = 0;
  resizes.length = 0;
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('InspectMetrics', () => {
  it('draws a chart per metric and reports the peaks', async () => {
    stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(await screen.findAllByTestId('metric-chart')).toHaveLength(2);
    expect(screen.getByText('peak 50m')).toBeInTheDocument();
    expect(screen.getByText('peak 381 MiB')).toBeInTheDocument();
  });

  it('names where the samples came from', async () => {
    stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(
      await screen.findByText('monitoring/prometheus-operated:9090 (https)'),
    ).toBeInTheDocument();
  });

  it('feeds the points into the chart', async () => {
    stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');

    await waitFor(() => {
      expect(updates.some((points) => points.length === 2)).toBe(true);
    });
  });

  it('defaults to an hour and refetches when the range changes', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');
    expect(String(fetchMock.mock.calls[0][0])).toContain('range=1h');

    await user.selectOptions(screen.getByLabelText('Metric range'), '24h');

    await waitFor(() => {
      const calls = fetchMock.mock.calls;
      expect(String(calls[calls.length - 1][0])).toContain('range=24h');
    });
  });

  it('says so when prometheus has no samples for the pod', async () => {
    stub(history({ cpu: [], memory: [] }));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(await screen.findByText(/no samples for this pod/)).toBeInTheDocument();
    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();
  });

  it('surfaces a failure instead of an empty chart', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ message: 'prometheus is unavailable' }),
      }),
    );
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(await screen.findByText('prometheus is unavailable')).toBeInTheDocument();
    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(await screen.findByText('loading metrics failed')).toBeInTheDocument();
  });

  it('resizes the chart when the drawer width changes', async () => {
    const callbacks: ResizeObserverCallback[] = [];
    class Observer {
      constructor(callback: ResizeObserverCallback) {
        callbacks.push(callback);
      }

      observe(): void {
        return undefined;
      }

      unobserve(): void {
        return undefined;
      }

      disconnect(): void {
        return undefined;
      }
    }
    vi.stubGlobal('ResizeObserver', Observer);
    stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');

    callbacks.forEach((callback) => {
      callback([], {} as ResizeObserver);
    });

    expect(resizes).toHaveLength(callbacks.length);
  });

  it('drops an answer that lands after unmount', () => {
    const deferred = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            deferred.settle = () => {
              resolve({ ok: true, json: () => Promise.resolve(history()) });
            };
          }),
      ),
    );
    const view = render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    view.unmount();
    deferred.settle();

    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();
  });
});

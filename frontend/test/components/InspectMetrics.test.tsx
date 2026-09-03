import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectMetrics, { Chart } from '../../src/components/InspectMetrics';
import type { ChartColors, ChartHandle } from '../../src/lib/chart';
import type { MetricPoint } from '../../src/lib/types';
import { canvasColors } from '../../src/lib/themeColors';
import { BUILT_IN_THEMES, themeById } from '../../src/lib/theme';
import { useThemeStore } from '../../src/store/theme';
import { bumpClusterEpoch } from '../../src/store/cluster';

const createChart = vi.fn<(node: HTMLElement, options: unknown) => ChartHandle>();
const updates: MetricPoint[][] = [];
const resizes: number[] = [];
const colorChanges: ChartColors[] = [];

vi.mock('../../src/lib/chart', () => ({
  createChart: (node: HTMLElement, options: unknown) => createChart(node, options),
}));

function chartStub(): ChartHandle {
  return {
    update: (points: MetricPoint[]) => {
      updates.push(points);
    },
    setColors: (colors: ChartColors) => {
      colorChanges.push(colors);
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

  it('clears the previous range while its replacement is loading', async () => {
    const user = userEvent.setup();
    let finishNext!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishNext = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(history({ source: 'one-hour samples' })),
      })
      .mockImplementationOnce(() => next);
    vi.stubGlobal('fetch', fetchMock);
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    expect(await screen.findByText('one-hour samples')).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('Metric range'), '24h');
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(2);
    });

    expect(screen.queryByText('one-hour samples')).not.toBeInTheDocument();
    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();

    await act(async () => {
      finishNext({
        ok: true,
        json: () => Promise.resolve(history({ source: 'day samples' })),
      });
      await next;
    });
  });

  it('says so when prometheus has no samples for the pod', async () => {
    stub(history({ cpu: [], memory: [] }));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(await screen.findByText(/no samples for this pod/)).toBeInTheDocument();
    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();
  });

  it('says out loud when spinoza measured this itself', async () => {
    stub(history({ source: undefined, sampled: true, since: Date.now() - 12 * 60_000 }));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    const notice = await screen.findByTestId('sampled-notice');
    expect(notice).toHaveTextContent('found no Prometheus');
    expect(notice).toHaveTextContent('every 15 seconds');
    expect(notice).toHaveTextContent('Collected so far: 12 minutes.');
  });

  it('says nothing about measuring when a metrics database answered', async () => {
    stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');

    expect(screen.queryByTestId('sampled-notice')).not.toBeInTheDocument();
  });

  it('leaves the collected line off until there is something to count', async () => {
    stub(history({ source: undefined, sampled: true }));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    const notice = await screen.findByTestId('sampled-notice');
    expect(notice).not.toHaveTextContent('Collected so far');
  });

  it('offers only the spans it can reach back to once it is measuring', async () => {
    stub(history({ source: undefined, sampled: true }));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findByTestId('sampled-notice');

    const spans = await screen.findByLabelText('Metric range');
    expect(spans).toHaveTextContent('15m');
    expect(spans).toHaveTextContent('1h');
    expect(spans).not.toHaveTextContent('24h');
  });

  it('drops back to an hour when the span picked is one it cannot reach', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      const sampled = url.includes('range=24h');
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve(history({ source: undefined, sampled })),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');

    await user.selectOptions(screen.getByLabelText('Metric range'), '24h');

    await waitFor(() => {
      expect(screen.getByLabelText('Metric range')).toHaveValue('1h');
    });
  });

  it('tells a pod with nothing measured yet apart from one prometheus has forgotten', async () => {
    stub(history({ source: undefined, sampled: true, cpu: [], memory: [] }));
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    expect(await screen.findByText(/Nothing measured yet/)).toBeInTheDocument();
    expect(screen.queryByText(/no samples for this pod/)).not.toBeInTheDocument();
  });

  it('comes back for the readings taken while it is open', async () => {
    vi.useFakeTimers();
    try {
      const fetchMock = stub(history({ source: undefined, sampled: true }));
      render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
      await vi.waitFor(() => {
        expect(screen.getByTestId('sampled-notice')).toBeInTheDocument();
      });
      const before = fetchMock.mock.calls.length;

      await act(async () => {
        await vi.advanceTimersByTimeAsync(15_000);
      });

      expect(fetchMock.mock.calls.length).toBeGreaterThan(before);
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not overlap sampled metrics refreshes', async () => {
    vi.useFakeTimers();
    try {
      let finishRefresh!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
      const refresh = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
        finishRefresh = resolve;
      });
      const fetchMock = vi
        .fn()
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(history({ source: undefined, sampled: true })),
        })
        .mockImplementationOnce(() => refresh)
        .mockResolvedValue({
          ok: true,
          json: () => Promise.resolve(history({ source: undefined, sampled: true })),
        });
      vi.stubGlobal('fetch', fetchMock);
      render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
      await vi.waitFor(() => {
        expect(fetchMock).toHaveBeenCalledTimes(2);
      });

      await act(async () => {
        await vi.advanceTimersByTimeAsync(45000);
      });
      expect(fetchMock).toHaveBeenCalledTimes(2);

      await act(async () => {
        finishRefresh({
          ok: true,
          json: () => Promise.resolve(history({ source: undefined, sampled: true })),
        });
        await refresh;
        await Promise.resolve();
        await vi.advanceTimersByTimeAsync(15000);
      });
      expect(fetchMock).toHaveBeenCalledTimes(3);
    } finally {
      vi.useRealTimers();
    }
  });

  it('does not poll a metrics database that already holds the whole span', async () => {
    vi.useFakeTimers();
    try {
      const fetchMock = stub(history());
      render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
      await vi.waitFor(() => {
        expect(screen.getAllByTestId('metric-chart')).toHaveLength(2);
      });
      const before = fetchMock.mock.calls.length;

      await act(async () => {
        await vi.advanceTimersByTimeAsync(60_000);
      });

      expect(fetchMock.mock.calls.length).toBe(before);
    } finally {
      vi.useRealTimers();
    }
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

  it("clears the previous pod's charts before the next one loads", async () => {
    stub(history());
    const view = render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');

    view.rerender(<InspectMetrics namespace="monitoring" pod="loki-1" />);

    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();
  });

  it("clears the previous cluster's charts before the replacement loads", async () => {
    let finishNext!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishNext = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(history()) })
      .mockImplementationOnce(() => next);
    vi.stubGlobal('fetch', fetchMock);
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');

    act(() => {
      bumpClusterEpoch();
    });

    expect(screen.queryByTestId('metric-chart')).not.toBeInTheDocument();
    expect(
      screen.queryByText('monitoring/prometheus-operated:9090 (https)'),
    ).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      finishNext({ ok: true, json: () => Promise.resolve(history({ source: 'new cluster' })) });
      await next;
      await Promise.resolve();
    });
    expect(screen.getByText('new cluster')).toBeInTheDocument();
  });

  it('does not apply a metric answer from the previous cluster', async () => {
    let finishOld!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const old = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishOld = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockImplementationOnce(() => old)
        .mockResolvedValueOnce({
          ok: true,
          json: () => Promise.resolve(history({ source: 'new cluster' })),
        }),
    );
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    act(() => {
      bumpClusterEpoch();
    });
    expect(await screen.findByText('new cluster')).toBeInTheDocument();
    await act(async () => {
      finishOld({
        ok: true,
        json: () => Promise.resolve(history({ source: 'old cluster' })),
      });
      await old;
      await Promise.resolve();
    });

    expect(screen.queryByText('old cluster')).not.toBeInTheDocument();
    expect(screen.getByText('new cluster')).toBeInTheDocument();
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

  it('drops a failure that lands after unmount', () => {
    const deferred = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            deferred.settle = () => {
              reject(new Error('the backend went away'));
            };
          }),
      ),
    );
    const view = render(<InspectMetrics namespace="monitoring" pod="loki-0" />);

    view.unmount();
    deferred.settle();

    expect(screen.queryByText('the backend went away')).not.toBeInTheDocument();
  });
});

describe('a chart whose colours change', () => {
  it('recolours in place instead of building a new chart', () => {
    const view = render(
      <Chart
        points={[{ at: 100, value: 1 }]}
        stroke="#0ea5e9"
        fill="#082f49"
        axis="#737373"
        grid="#262626"
        format={String}
        metric="cpu"
      />,
    );

    view.rerender(
      <Chart
        points={[{ at: 100, value: 1 }]}
        stroke="#22c55e"
        fill="#052e16"
        axis="#737373"
        grid="#262626"
        format={String}
        metric="cpu"
      />,
    );

    expect(createChart).toHaveBeenCalledTimes(1);
    expect(colorChanges.at(-1)).toEqual({
      stroke: '#22c55e',
      fill: '#052e16',
      axis: '#737373',
      grid: '#262626',
    });
  });

  it('recolours the whole chart when the app theme changes', async () => {
    stub(history());
    render(<InspectMetrics namespace="monitoring" pod="loki-0" />);
    await screen.findAllByTestId('metric-chart');
    const built = createChart.mock.calls.length;
    colorChanges.length = 0;

    act(() => {
      useThemeStore.getState().setPreference('light');
    });

    const light = canvasColors(themeById(BUILT_IN_THEMES, 'light'));
    expect(createChart).toHaveBeenCalledTimes(built);
    expect(colorChanges.at(-1)).toEqual({
      stroke: light.memoryStroke,
      fill: light.memoryFill,
      axis: light.chartAxis,
      grid: light.chartGrid,
    });

    act(() => {
      useThemeStore.getState().setPreference('dark');
    });
  });

  it('never leaves a rebuilt chart empty waiting for the next poll', () => {
    const points = [
      { at: 100, value: 1 },
      { at: 160, value: 2 },
    ];
    const view = render(
      <Chart
        points={points}
        stroke="#0ea5e9"
        fill="#082f49"
        axis="#737373"
        grid="#262626"
        format={String}
        metric="cpu"
      />,
    );
    updates.length = 0;

    view.rerender(
      <Chart
        points={points}
        stroke="#0ea5e9"
        fill="#082f49"
        axis="#737373"
        grid="#262626"
        format={String}
        metric="memory"
      />,
    );

    expect(createChart).toHaveBeenCalledTimes(2);
    expect(updates.at(-1)).toEqual(points);
  });
});

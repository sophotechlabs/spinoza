import { beforeEach, describe, expect, it, vi } from 'vitest';

interface FakeAxis {
  stroke?: unknown;
  grid?: { stroke?: unknown; width?: number };
  ticks?: { stroke?: unknown; size?: number };
}

interface FakeSeries {
  stroke?: unknown;
  fill?: unknown;
}

const spies = vi.hoisted(() => ({
  redraw: vi.fn(),
  setData: vi.fn(),
  instances: [] as { series: unknown[]; axes: unknown[] }[],
}));

vi.mock('uplot', () => {
  class FakeUPlot {
    series: FakeSeries[];
    axes: FakeAxis[];

    constructor(opts: { axes: FakeAxis[]; series: FakeSeries[] }) {
      this.series = opts.series.map((s) => ({ ...s }));
      this.axes = opts.axes.map((a) => ({ ...a, grid: { ...a.grid }, ticks: { ...a.ticks } }));
      spies.instances.push(this);
    }

    setData(...args: unknown[]) {
      spies.setData(...args);
    }

    redraw(...args: unknown[]) {
      spies.redraw(...args);
    }

    setSize() {
      return undefined;
    }

    destroy() {
      return undefined;
    }
  }
  return { default: FakeUPlot };
});

vi.mock('uplot/dist/uPlot.min.css', () => ({}));

import { createChart } from '../../src/lib/chart';

const DARK = { stroke: '#4ade80', fill: '#052e16', axis: '#737373', grid: '#262626' };
const LIGHT = { stroke: '#16a34a', fill: '#dcfce7', axis: '#525252', grid: '#e5e5e5' };

function build() {
  const node = document.createElement('div');
  return createChart(node, { colors: DARK, format: String, metric: 'cpu' });
}

function internals() {
  const last = spies.instances[spies.instances.length - 1];
  return last as unknown as { series: FakeSeries[]; axes: FakeAxis[] };
}

beforeEach(() => {
  spies.redraw.mockReset();
  spies.setData.mockReset();
  spies.instances.length = 0;
});

describe('recolouring a chart uPlot has already built', () => {
  it('hands uPlot callables, because it calls stroke as a function while drawing', () => {
    const chart = build();

    chart.setColors(LIGHT);

    const inner = internals();
    expect(typeof inner.series[1].stroke).toBe('function');
    expect(typeof inner.series[1].fill).toBe('function');
    for (const axis of inner.axes) {
      expect(typeof axis.stroke).toBe('function');
      expect(typeof axis.grid?.stroke).toBe('function');
      expect(typeof axis.ticks?.stroke).toBe('function');
    }
  });

  it('resolves those callables to the colours it was given', () => {
    const chart = build();

    chart.setColors(LIGHT);

    const inner = internals();
    const seriesStroke = inner.series[1].stroke as () => string;
    const seriesFill = inner.series[1].fill as () => string;
    const axisStroke = inner.axes[0].stroke as () => string;
    const gridStroke = inner.axes[0].grid?.stroke as () => string;
    expect(seriesStroke()).toBe(LIGHT.stroke);
    expect(seriesFill()).toBe(LIGHT.fill);
    expect(axisStroke()).toBe(LIGHT.axis);
    expect(gridStroke()).toBe(LIGHT.grid);
  });

  it('repaints the paths and axes instead of clearing the canvas', () => {
    const chart = build();

    chart.setColors(LIGHT);

    expect(spies.redraw).toHaveBeenCalledTimes(1);
    expect(spies.redraw).toHaveBeenCalledWith();
  });

  it('keeps the rest of the grid configuration', () => {
    const chart = build();

    chart.setColors(LIGHT);

    expect(internals().axes[0].grid?.width).toBe(1);
  });
});

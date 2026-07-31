import uPlot from 'uplot';
import 'uplot/dist/uPlot.min.css';
import type { MetricPoint } from './types';

export interface ChartHandle {
  update: (points: MetricPoint[]) => void;
  resize: (width: number) => void;
  destroy: () => void;
}

interface ChartOptions {
  stroke: string;
  fill: string;
  format: (value: number) => string;
  metric: 'cpu' | 'memory';
}

const MIB = 1024 * 1024;

const cpuTicks = [0.001, 0.002, 0.005, 0.01, 0.02, 0.05, 0.1, 0.2, 0.5, 1, 2, 5, 10, 20, 50];

const memoryTicks = [
  MIB,
  2 * MIB,
  5 * MIB,
  10 * MIB,
  25 * MIB,
  50 * MIB,
  100 * MIB,
  250 * MIB,
  500 * MIB,
  1024 * MIB,
  2048 * MIB,
  5120 * MIB,
];

function ticksFor(metric: 'cpu' | 'memory'): number[] {
  if (metric === 'cpu') {
    return cpuTicks;
  }
  return memoryTicks;
}

function series(points: MetricPoint[]): uPlot.AlignedData {
  return [points.map((point) => point.at), points.map((point) => point.value)];
}

export function createChart(node: HTMLElement, options: ChartOptions): ChartHandle {
  const chart = new uPlot(
    {
      width: node.clientWidth || 320,
      height: 130,
      padding: [8, 8, 0, 0],
      cursor: { y: false },
      legend: { show: false },
      axes: [
        { stroke: '#737373', grid: { stroke: '#262626', width: 1 }, ticks: { stroke: '#262626' } },
        {
          stroke: '#737373',
          grid: { stroke: '#262626', width: 1 },
          ticks: { stroke: '#262626' },
          size: 58,
          incrs: ticksFor(options.metric),
          values: (_self, splits) => splits.map((split) => options.format(split)),
        },
      ],
      series: [
        { value: (_self, raw) => new Date(raw * 1000).toLocaleTimeString() },
        {
          stroke: options.stroke,
          fill: options.fill,
          width: 1.5,
          value: (_self, raw) => options.format(raw),
        },
      ],
    },
    series([]),
    node,
  );

  return {
    update: (next: MetricPoint[]) => {
      chart.setData(series(next));
    },
    resize: (width: number) => {
      chart.setSize({ width, height: 130 });
    },
    destroy: () => {
      chart.destroy();
    },
  };
}

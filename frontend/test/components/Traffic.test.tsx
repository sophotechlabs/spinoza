import type { ReactNode } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import dagre from '@dagrejs/dagre';
import type { TrafficEdge, TrafficGraph, TrafficNode } from '../../src/lib/types';
import type { TrafficFlowNode } from '../../src/lib/trafficLayout';

vi.mock('@xyflow/react', () => {
  const ReactFlowStub = ({
    nodes,
    edges,
    colorMode,
    children,
  }: {
    nodes: TrafficFlowNode[];
    edges: { id: string; label?: string }[];
    colorMode?: string;
    children?: ReactNode;
  }) => (
    <div data-testid="react-flow" data-color-mode={colorMode}>
      {nodes.map((node) => (
        <span key={node.id}>{node.data.label}</span>
      ))}
      {edges.map((edge) => (
        <span key={edge.id} data-testid="edge">
          {edge.label}
        </span>
      ))}
      {children}
    </div>
  );
  const Background = () => <div data-testid="background" />;
  const Controls = () => <div data-testid="controls" />;
  return { ReactFlow: ReactFlowStub, Background, Controls };
});

import Traffic from '../../src/components/Traffic';
import { useThemeStore } from '../../src/store/theme';
import { bumpClusterEpoch } from '../../src/store/cluster';

function node(id: string): TrafficNode {
  const [namespace, workload] = id.split('/');
  return { id, namespace, workload };
}

function edge(over: Partial<TrafficEdge> = {}): TrafficEdge {
  return { from: 'apps/web', to: 'apps/api', rate: 9, dropped: 0, ...over };
}

function graph(over: Partial<TrafficGraph> = {}): TrafficGraph {
  return {
    source: 'Cilium Hubble',
    nodes: [node('apps/web'), node('apps/api')],
    edges: [edge()],
    ...over,
  };
}

function stub(body: TrafficGraph): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(body) }),
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
  act(() => {
    useThemeStore.getState().setPreference('dark');
  });
});

describe('Traffic', () => {
  it('shows a loading state before the graph resolves', async () => {
    stub(graph({ nodes: [], edges: [] }));
    render(<Traffic />);
    expect(screen.getByText('Loading the traffic graph')).toBeInTheDocument();
    expect(
      await screen.findByText('No workload-to-workload traffic in the last five minutes.'),
    ).toBeInTheDocument();
  });

  it('draws the workloads, the rate on the edge and the mesh it read', async () => {
    stub(graph());
    render(<Traffic />);

    expect(await screen.findByText('apps/web')).toBeInTheDocument();
    expect(screen.getByText('apps/api')).toBeInTheDocument();
    expect(screen.getByTestId('edge')).toHaveTextContent('9.00/s');
    expect(screen.getByText('Cilium Hubble')).toBeInTheDocument();
    expect(screen.getByTestId('background')).toBeInTheDocument();
    expect(screen.getByTestId('controls')).toBeInTheDocument();
  });

  it('names both edge colours in the legend', async () => {
    stub(graph());
    render(<Traffic />);
    await screen.findByText('apps/web');

    expect(screen.getByText('Forwarded flows per second')).toBeInTheDocument();
    expect(screen.getByText('Some flows dropped')).toBeInTheDocument();
  });

  it('refuses to draw more workloads than the node cap', async () => {
    const nodes = Array.from({ length: 401 }, (_unused, index) => node(`apps/w-${String(index)}`));
    stub(graph({ nodes, edges: [] }));
    render(<Traffic />);

    expect(await screen.findByText(/401 workloads/)).toBeInTheDocument();
    expect(screen.queryByTestId('react-flow')).not.toBeInTheDocument();
  });

  it('shows what to configure when the graph came back with only an error', async () => {
    stub(graph({ nodes: [], edges: [], error: 'Add flow:labelsContext to the Cilium values' }));
    render(<Traffic />);

    expect(await screen.findByText('The traffic graph could not be loaded')).toBeInTheDocument();
    expect(screen.getByText(/flow:labelsContext/)).toBeInTheDocument();
  });

  it('warns above the graph when some of it is missing', async () => {
    stub(graph({ error: 'one mesh answered late' }));
    render(<Traffic />);

    expect(await screen.findByText(/one mesh answered late/)).toBeInTheDocument();
    expect(screen.getByTestId('react-flow')).toBeInTheDocument();
  });

  it('shows the error message when the request rejects', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('traffic endpoint is down')));
    render(<Traffic />);

    expect(await screen.findByText('traffic endpoint is down')).toBeInTheDocument();
  });

  it('shows a generic message when the request rejects with a non-error', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('boom'));
    render(<Traffic />);

    expect(await screen.findByText('traffic graph request failed')).toBeInTheDocument();
  });

  it('says the graph stopped updating once a later poll fails', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(graph()) });
        }
        return Promise.reject(new Error('traffic endpoint is down'));
      }),
    );

    render(<Traffic />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId('react-flow')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('traffic endpoint is down');
  });

  it('follows the app theme', async () => {
    stub(graph());
    render(<Traffic />);
    await screen.findByText('apps/web');

    expect(screen.getByTestId('react-flow')).toHaveAttribute('data-color-mode', 'dark');

    act(() => {
      useThemeStore.getState().setPreference('light');
    });

    expect(screen.getByTestId('react-flow')).toHaveAttribute('data-color-mode', 'light');
  });
});

describe('a poll that brings back the same traffic', () => {
  it('does not lay the workloads out again', async () => {
    vi.useFakeTimers();
    const body = graph();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve({ ok: true, json: () => Promise.resolve(body) })),
    );
    const layout = vi.spyOn(dagre, 'layout');

    render(<Traffic />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(layout).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15000);
    });

    expect(layout).toHaveBeenCalledTimes(1);
    layout.mockRestore();
  });

  it('moves the rate without laying the workloads out again', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        let rate = 9;
        if (call > 1) {
          rate = 11;
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(graph({ edges: [edge({ rate })] })),
        });
      }),
    );
    const layout = vi.spyOn(dagre, 'layout');

    render(<Traffic />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByTestId('edge')).toHaveTextContent('9.00/s');

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(screen.getByTestId('edge')).toHaveTextContent('11.0/s');
    expect(layout).toHaveBeenCalledTimes(1);
    layout.mockRestore();
  });

  it('lays out again once a workload appears', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve(graph()) });
        }
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve(
              graph({
                nodes: [node('apps/web'), node('apps/api'), node('apps/beat')],
                edges: [edge(), edge({ from: 'apps/beat' })],
              }),
            ),
        });
      }),
    );
    const layout = vi.spyOn(dagre, 'layout');

    render(<Traffic />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });

    expect(layout).toHaveBeenCalledTimes(2);
    layout.mockRestore();
  });
});

describe('a change of cluster', () => {
  it('drops the graph the previous cluster drew', async () => {
    stub(graph());
    render(<Traffic />);
    expect(await screen.findByText('apps/web')).toBeInTheDocument();

    act(() => {
      bumpClusterEpoch();
    });

    expect(screen.queryByText('apps/web')).not.toBeInTheDocument();
    expect(screen.getByText('Loading the traffic graph')).toBeInTheDocument();
  });
});

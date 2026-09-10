import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Waste from '../../src/components/Waste';
import type { WasteReport, WasteRow } from '../../src/lib/types';
import { useNamespaceStore } from '../../src/store/namespace';

function row(extra: Partial<WasteRow> = {}): WasteRow {
  return {
    namespace: 'prod',
    cpuRequested: 1000,
    cpuUsed: 250,
    cpuReclaimable: 750,
    memRequested: 2048,
    memUsed: 512,
    memReclaimable: 1536,
    measured: true,
    ...extra,
  };
}

const report: WasteReport = {
  namespaces: [row(), row({ namespace: 'shop', cpuReclaimable: 100 })],
  workloads: [row({ kind: 'Deployment', name: 'web' })],
  window: 'the last 30 minutes',
  source: 'prometheus',
  measured: true,
};

function stub(body: unknown, ok = true) {
  const asked: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      asked.push(url);
      return Promise.resolve({
        ok,
        status: ok ? 200 : 500,
        json: () => Promise.resolve(body),
        text: () => Promise.resolve('{}'),
      });
    }),
  );
  return asked;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Waste', () => {
  it('ranks namespaces by what they hold and never use', async () => {
    stub(report);

    render(<Waste />);

    expect(await screen.findByRole('columnheader', { name: 'Namespace' })).toBeInTheDocument();
    expect(screen.getByText('prod')).toBeInTheDocument();
    expect(screen.getByText('shop')).toBeInTheDocument();
    expect(screen.getByText('usage from prometheus')).toBeInTheDocument();
  });

  it('groups by workload when asked', async () => {
    stub(report);

    render(<Waste />);
    await screen.findByText('prod');
    await userEvent.click(screen.getByRole('button', { name: 'By workload' }));

    expect(await screen.findByRole('columnheader', { name: 'Workload' })).toBeInTheDocument();
    expect(screen.getByText('prod/web')).toBeInTheDocument();
    expect(screen.getByText('Deployment')).toBeInTheDocument();
  });

  it('narrows the rows to what the filter names', async () => {
    stub(report);

    render(<Waste />);
    await screen.findByText('prod');
    await userEvent.type(screen.getByLabelText('Filter rows'), 'shop');

    await waitFor(() => {
      expect(screen.queryByText('prod')).not.toBeInTheDocument();
    });
    expect(screen.getByText('shop')).toBeInTheDocument();
    expect(screen.getByText('1 rows')).toBeInTheDocument();
  });

  it('says nothing measured usage rather than drawing a zero', async () => {
    stub({
      namespaces: [row({ measured: false })],
      workloads: [],
      measured: false,
      reason: 'nothing measured usage',
    });

    render(<Waste />);

    expect(await screen.findByText(/nothing measured usage/)).toBeInTheDocument();
    expect(screen.getAllByText('not measured').length).toBeGreaterThan(0);
    expect(screen.getAllByText('—').length).toBeGreaterThan(0);
  });

  it('names what it could not read', async () => {
    stub({ ...report, partialOn: ['payments', 'shop/api'] });

    render(<Waste />);

    expect(
      await screen.findByText(/usage was not read for payments, shop\/api/),
    ).toBeInTheDocument();
  });

  it('says so when nothing here reserves anything', async () => {
    stub({ namespaces: [], workloads: [], measured: true });

    render(<Waste />);

    expect(await screen.findByText('Nothing here reserves CPU or memory.')).toBeInTheDocument();
  });

  it('names what went wrong rather than an empty table', async () => {
    stub({ message: 'this view reads the whole cluster' }, false);

    render(<Waste />);

    expect(await screen.findByText(/this view reads the whole cluster/)).toBeInTheDocument();
  });

  it('opens the kind a row names, scoped to its namespace', async () => {
    stub(report);
    const opened: { namespace: string; kind: string }[] = [];

    render(
      <Waste
        onOpenScope={(namespace, kind) => {
          opened.push({ namespace, kind });
        }}
      />,
    );
    await screen.findByText('prod');
    await userEvent.click(screen.getByRole('button', { name: 'prod' }));

    expect(opened).toEqual([{ namespace: 'prod', kind: '' }]);
  });

  it('carries the workload kind when a workload row is opened', async () => {
    stub(report);
    const opened: { namespace: string; kind: string }[] = [];

    render(
      <Waste
        onOpenScope={(namespace, kind) => {
          opened.push({ namespace, kind });
        }}
      />,
    );
    await screen.findByText('prod');
    await userEvent.click(screen.getByRole('button', { name: 'By workload' }));
    await userEvent.click(await screen.findByRole('button', { name: 'prod/web' }));

    expect(opened).toEqual([{ namespace: 'prod', kind: 'Deployment' }]);
  });
  it('scopes to the namespace the workspace is on', async () => {
    const asked = stub(report);
    act(() => {
      useNamespaceStore.getState().choose('payments');
    });

    render(<Waste />);

    await screen.findByText('prod');
    expect(asked.some((one) => one.includes('namespace=payments'))).toBe(true);
    expect(screen.getByText(/payments/)).toBeInTheDocument();
    act(() => {
      useNamespaceStore.getState().choose('');
    });
  });

  it('reads the report once rather than on every render', async () => {
    const asked = stub(report);

    render(<Waste />);
    await screen.findByText('prod');
    const after = asked.length;
    await waitFor(() => {
      expect(asked.length).toBe(after);
    });

    expect(after).toBe(1);
  });
});

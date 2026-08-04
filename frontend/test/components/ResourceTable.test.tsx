import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Column, ResourceDescriptor, Row } from '../../src/lib/types';
import ResourceTable from '../../src/components/ResourceTable';
import { useResourcesStore } from '../../src/store/resources';
import { makeColumns, makeDescriptor, makeRow } from '../helpers';

const SUB = 's1';
const descriptor = makeDescriptor({ resource: 'pods', kind: 'Pod' });

function seed(columns: Column[], namespaced: boolean, rows: Row[]): void {
  useResourcesStore.getState().applySnapshot(SUB, columns, namespaced, rows);
}

function renderTable(active: ResourceDescriptor | null, selected: Row | null, onSelect = vi.fn()) {
  return render(
    <ResourceTable active={active} subId={SUB} selected={selected} onSelect={onSelect} />,
  );
}

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map(), errors: new Map() });
}

describe('ResourceTable', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    resetStore();
  });

  it('shows why the resource could not be loaded', () => {
    useResourcesStore
      .getState()
      .failSub(SUB, 'pods is forbidden: User "spinoza" cannot list resource "pods"');

    renderTable(descriptor, null);

    expect(screen.getByText('Pod could not be loaded')).toBeInTheDocument();
    expect(screen.getByText(/cannot list resource/)).toBeInTheDocument();
  });

  it('does not pretend the resource is empty while it is failing', () => {
    seed(makeColumns(['Ready']), true, []);
    useResourcesStore.getState().failSub(SUB, 'the cluster did not answer in time');

    renderTable(descriptor, null);

    expect(screen.queryByLabelText('Filter by name')).not.toBeInTheDocument();
    expect(screen.getByText('the cluster did not answer in time')).toBeInTheDocument();
  });

  it('goes back to the table once a snapshot arrives', () => {
    useResourcesStore.getState().failSub(SUB, 'boom');
    seed(makeColumns(['Ready']), true, [makeRow({ uid: 'a', name: 'alpha' })]);

    renderTable(descriptor, null);

    expect(screen.queryByText('boom')).not.toBeInTheDocument();
    expect(screen.getByText('alpha')).toBeInTheDocument();
  });

  it('clears a namespace filter when the resource changes', async () => {
    const user = userEvent.setup();
    seed(makeColumns(['Ready']), true, [
      makeRow({ uid: 'a', name: 'alpha', namespace: 'one' }),
      makeRow({ uid: 'b', name: 'bravo', namespace: 'two' }),
    ]);
    const view = renderTable(descriptor, null);
    await user.selectOptions(screen.getByLabelText('Namespace'), 'two');
    expect(screen.queryByText('alpha')).not.toBeInTheDocument();

    useResourcesStore
      .getState()
      .applySnapshot('s2', makeColumns(['Ready']), false, [makeRow({ uid: 'c', name: 'charlie' })]);
    view.rerender(
      <ResourceTable
        active={makeDescriptor({ resource: 'nodes', kind: 'Node', namespaced: false })}
        subId="s2"
        selected={null}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByText('charlie')).toBeInTheDocument();
  });

  it('clears the name filter when the resource changes', async () => {
    const user = userEvent.setup();
    seed(makeColumns(['Ready']), true, [makeRow({ uid: 'a', name: 'alpha' })]);
    const view = renderTable(descriptor, null);
    await user.type(screen.getByLabelText('Filter by name'), 'zzz');
    expect(screen.queryByText('alpha')).not.toBeInTheDocument();

    useResourcesStore
      .getState()
      .applySnapshot('s2', makeColumns(['Ready']), true, [makeRow({ uid: 'c', name: 'charlie' })]);
    view.rerender(
      <ResourceTable active={descriptor} subId="s2" selected={null} onSelect={vi.fn()} />,
    );

    expect(screen.getByLabelText('Filter by name')).toHaveValue('');
    expect(screen.getByText('charlie')).toBeInTheDocument();
  });

  it('keeps the age column moving without a websocket event', async () => {
    vi.useFakeTimers();
    const created = new Date(Date.now() - 5000).toISOString();
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'alpha', createdAt: created })]);
    renderTable(descriptor, null);
    expect(screen.getByText('5s')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(31000);
    });

    expect(screen.queryByText('5s')).not.toBeInTheDocument();
    expect(screen.getByText('35s')).toBeInTheDocument();
    vi.useRealTimers();
  });

  it('renders a placeholder when no resource is active', () => {
    renderTable(null, null);
    expect(screen.getByText('Select a resource to view.')).toBeInTheDocument();
  });

  it('says it is still waiting rather than showing an empty table', () => {
    renderTable(descriptor, null);

    expect(screen.getByText('Loading Pod…')).toBeInTheDocument();
    expect(screen.queryByText('This cluster has no Pod objects.')).not.toBeInTheDocument();
  });

  it('says the resource is genuinely empty once the snapshot lands', () => {
    seed(makeColumns(['Ready']), true, []);

    renderTable(descriptor, null);

    expect(screen.getByText('This cluster has no Pod objects.')).toBeInTheDocument();
    expect(screen.queryByText('Loading Pod…')).not.toBeInTheDocument();
  });

  it('separates a filter with no match from an empty resource', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    renderTable(descriptor, null);

    await user.type(screen.getByLabelText('Filter by name'), 'zzz');

    expect(screen.getByText('Nothing matches the current filter.')).toBeInTheDocument();
    expect(screen.queryByText('This cluster has no Pod objects.')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear filter' }));

    expect(screen.getByRole('button', { name: 'pod-a' })).toBeInTheDocument();
    expect(screen.getByLabelText('Filter by name')).toHaveValue('');
  });

  it('drops a namespace filter whose namespace is gone instead of filtering to nothing', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'temp' }),
    ]);
    renderTable(descriptor, null);
    await user.selectOptions(screen.getByLabelText('Namespace'), 'temp');
    expect(screen.queryByRole('button', { name: 'pod-a' })).not.toBeInTheDocument();

    act(() => {
      useResourcesStore.getState().applyDeltas(SUB, [{ type: 'deleted', subId: SUB, uid: 'b' }]);
    });

    expect(screen.getByLabelText('Namespace')).toHaveValue('');
    expect(screen.getByRole('button', { name: 'pod-a' })).toBeInTheDocument();
  });

  it('keeps a namespace filter while the snapshot has not arrived yet', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    renderTable(descriptor, null);
    await user.selectOptions(screen.getByLabelText('Namespace'), 'prod');

    act(() => {
      useResourcesStore.setState({ subs: new Map(), errors: new Map() });
    });

    expect(screen.getByText('Loading Pod…')).toBeInTheDocument();
  });

  it('renders Name, snapshot columns and Age for a cluster-scoped resource', () => {
    seed(makeColumns(['Ready', 'Status']), false, []);
    renderTable(descriptor, null);
    const headers = screen.getAllByRole('columnheader').map((cell) => cell.textContent);
    expect(headers).toEqual(['Name', 'Ready', 'Status', 'Age']);
  });

  it('inserts a Namespace column for a namespaced resource', () => {
    seed(makeColumns(['Ready']), true, []);
    renderTable(descriptor, null);
    const headers = screen.getAllByRole('columnheader').map((cell) => cell.textContent);
    expect(headers).toEqual(['Name', 'Namespace', 'Ready', 'Age']);
  });

  it('renders each row with its cells positionally', async () => {
    seed(makeColumns(['Ready', 'Status']), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: ['1/1', 'Running'] }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod', cells: ['0/1', 'Pending'] }),
    ]);
    renderTable(descriptor, null);
    expect(await screen.findByRole('button', { name: 'pod-a' })).toBeInTheDocument();
    expect(screen.getByText('1/1')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
    expect(screen.getByText('0/1')).toBeInTheDocument();
    expect(screen.getByText('Pending')).toBeInTheDocument();
  });

  it('renders container squares, colored ratios and restart counts by render hint', async () => {
    seed(
      [
        { name: 'Containers', render: 'containers' },
        { name: 'Ready', render: 'ratio' },
        { name: 'Restarts', render: 'restarts' },
      ],
      true,
      [
        makeRow({
          uid: 'a',
          name: 'pod-a',
          namespace: 'prod',
          cells: ['1/2', '1/2', '7'],
          containers: [
            { name: 'app', state: 'running', ready: true, restarts: 0, init: false },
            {
              name: 'sidecar',
              state: 'waiting',
              reason: 'CrashLoopBackOff',
              ready: false,
              restarts: 7,
              init: false,
            },
          ],
        }),
      ],
    );
    renderTable(descriptor, null);
    await screen.findByRole('button', { name: 'pod-a' });
    expect(screen.getByTitle('app: running')).toBeInTheDocument();
    expect(
      screen.getByTitle('sidecar: waiting (CrashLoopBackOff) · 7 restarts'),
    ).toBeInTheDocument();
    const ratio = screen.getByText('1/2');
    expect(ratio.className).toContain('text-warn');
    const restarts = screen.getByText('7');
    expect(restarts.className).toContain('text-error');
  });

  it('colors the status column by phase', async () => {
    seed([{ name: 'Status', render: 'status' }], true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: ['Running'] }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod', cells: ['CrashLoopBackOff'] }),
    ]);
    renderTable(descriptor, null);
    await screen.findByRole('button', { name: 'pod-a' });

    expect(screen.getByText('Running').className).toContain('text-ok');
    expect(screen.getByText('CrashLoopBackOff').className).toContain('text-error');
  });

  it('falls back to the cell text when a container column has no container data', async () => {
    seed([{ name: 'Containers', render: 'containers' }], true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: ['0/1'] }),
    ]);
    renderTable(descriptor, null);
    await screen.findByRole('button', { name: 'pod-a' });
    expect(screen.getByText('0/1')).toBeInTheDocument();
  });

  it('gives each column a width and a drag-to-resize handle', async () => {
    seed(makeColumns(['Ready']), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    const { container } = renderTable(descriptor, null);
    await screen.findByRole('button', { name: 'pod-a' });
    const nameHeader = screen.getAllByRole('columnheader')[0];
    expect(nameHeader.getAttribute('style')).toContain('width');
    expect(container.querySelectorAll('.cursor-col-resize').length).toBeGreaterThan(0);
  });

  it('renders an empty cell when a row has fewer cells than columns', async () => {
    seed(makeColumns(['Ready', 'Status']), false, [
      makeRow({ uid: 'a', name: 'pod-a', cells: ['1/1'] }),
    ]);
    renderTable(descriptor, null);
    const button = await screen.findByRole('button', { name: 'pod-a' });
    const row = button.closest('tr');
    if (!row) {
      throw new Error('row element not found');
    }
    const cells = row.querySelectorAll('td');
    expect(cells.item(2).textContent).toBe('');
  });

  it('formats age for seconds, minutes, hours and days', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-24T12:00:00Z'));
    seed(makeColumns([]), false, [
      makeRow({ uid: 's', name: 'sec', createdAt: '2026-07-24T11:59:30Z' }),
      makeRow({ uid: 'm', name: 'min', createdAt: '2026-07-24T11:55:00Z' }),
      makeRow({ uid: 'h', name: 'hour', createdAt: '2026-07-24T09:00:00Z' }),
      makeRow({ uid: 'd', name: 'day', createdAt: '2026-07-22T12:00:00Z' }),
    ]);
    renderTable(descriptor, null);
    expect(screen.getByText('30s')).toBeInTheDocument();
    expect(screen.getByText('5m')).toBeInTheDocument();
    expect(screen.getByText('3h')).toBeInTheDocument();
    expect(screen.getByText('2d')).toBeInTheDocument();
  });

  it('clamps a future createdAt to zero seconds', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-24T12:00:00Z'));
    seed(makeColumns([]), false, [
      makeRow({ uid: 'f', name: 'future', createdAt: '2026-07-24T12:01:40Z' }),
    ]);
    renderTable(descriptor, null);
    expect(screen.getByText('0s')).toBeInTheDocument();
  });

  it('renders an empty age for an unparseable createdAt', async () => {
    seed(makeColumns([]), false, [
      makeRow({ uid: 'bad', name: 'bad-row', createdAt: 'not-a-date' }),
    ]);
    renderTable(descriptor, null);
    const button = await screen.findByRole('button', { name: 'bad-row' });
    const row = button.closest('tr');
    if (!row) {
      throw new Error('row element not found');
    }
    const cells = row.querySelectorAll('td');
    expect(cells.item(1).textContent).toBe('');
  });

  it('virtualizes large row sets, rendering only a windowed subset', async () => {
    const rows = Array.from({ length: 200 }, (_, index) =>
      makeRow({
        uid: String(index),
        name: `row-${String(index).padStart(3, '0')}`,
        namespace: 'ns',
      }),
    );
    seed(makeColumns([]), true, rows);
    renderTable(descriptor, null);
    await screen.findByRole('button', { name: 'row-000' });
    const rendered = screen.getAllByRole('button', { name: /^row-\d/ });
    expect(rendered.length).toBeLessThan(200);
    expect(rendered.length).toBeGreaterThan(0);
  });

  it('toggles sorting through ascending, descending and none', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: '1', name: 'gamma', namespace: 'ns' }),
      makeRow({ uid: '2', name: 'alpha', namespace: 'ns' }),
    ]);
    renderTable(descriptor, null);
    const header = screen.getAllByRole('columnheader')[0];
    expect(header).toHaveAttribute('aria-sort', 'none');
    const headerButton = within(header).getByRole('button');
    await user.click(headerButton);
    expect(header).toHaveAttribute('aria-sort', 'ascending');
    await user.click(headerButton);
    expect(header).toHaveAttribute('aria-sort', 'descending');
    await user.click(headerButton);
    expect(header).toHaveAttribute('aria-sort', 'none');
  });

  it('calls onSelect with the row when its name is clicked', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: [] });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, null, onSelect);
    await user.click(await screen.findByRole('button', { name: 'pod-a' }));
    expect(onSelect).toHaveBeenCalledWith(row);
  });

  it('marks the selected row', async () => {
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, row);
    const button = await screen.findByRole('button', { name: 'pod-a' });
    const tr = button.closest('tr');
    if (!tr) {
      throw new Error('row element not found');
    }
    expect(tr.className).toContain('bg-surface-active');
  });

  it('shows pod CPU and memory from metrics, with a dash when a pod is missing', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            pods: { 'prod/pod-a': { cpuMilli: 150, memoryMi: 192, cpuPercent: 0, memPercent: 0 } },
            nodes: {},
          }),
      }),
    );
    seed(makeColumns(['Ready']), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);
    expect(await screen.findByText('150m')).toBeInTheDocument();
    expect(screen.getByText('192Mi')).toBeInTheDocument();
    expect(screen.getAllByText('—').length).toBeGreaterThan(0);
  });

  it('shows node CPU and memory usage bars from metrics', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            pods: {},
            nodes: { 'node-1': { cpuMilli: 1500, memoryMi: 2048, cpuPercent: 37, memPercent: 25 } },
          }),
      }),
    );
    const nodeDescriptor = makeDescriptor({ resource: 'nodes', kind: 'Node', namespaced: false });
    seed(makeColumns(['Status']), false, [
      makeRow({ uid: 'a', name: 'node-1', namespace: '' }),
      makeRow({ uid: 'b', name: 'node-2', namespace: '' }),
    ]);
    renderTable(nodeDescriptor, null);
    expect(await screen.findByText('37%')).toBeInTheDocument();
    expect(screen.getByText('25%')).toBeInTheDocument();
    expect(screen.getAllByText('0%').length).toBeGreaterThan(0);
  });

  it('says the metrics columns stopped updating without blanking them', async () => {
    vi.useFakeTimers();
    let call = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn(() => {
        call += 1;
        if (call === 1) {
          return Promise.resolve({
            ok: true,
            json: () =>
              Promise.resolve({
                pods: {
                  'prod/pod-a': { cpuMilli: 150, memoryMi: 192, cpuPercent: 0, memPercent: 0 },
                },
                nodes: {},
              }),
          });
        }
        return Promise.reject(new Error('metrics-server is down'));
      }),
    );
    seed(makeColumns(['Ready']), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    renderTable(descriptor, null);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('150m')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });

    expect(screen.getByRole('status')).toHaveTextContent('metrics-server is down');
    expect(screen.getByText('150m')).toBeInTheDocument();
  });
  it('filters rows by name as you type', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'web-1', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'api-1', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);
    expect(screen.getByText('2 of 2')).toBeInTheDocument();

    await user.type(screen.getByLabelText('Filter by name'), 'web');

    expect(screen.getByRole('button', { name: 'web-1' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'api-1' })).not.toBeInTheDocument();
    expect(screen.getByText('1 of 2')).toBeInTheDocument();
  });

  it('filters rows by namespace', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'web-1', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'web-2', namespace: 'staging' }),
    ]);
    renderTable(descriptor, null);

    await user.selectOptions(screen.getByLabelText('Namespace'), 'staging');

    expect(screen.getByRole('button', { name: 'web-2' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'web-1' })).not.toBeInTheDocument();
    expect(screen.getByText('1 of 2')).toBeInTheDocument();
  });

  it('hides the namespace picker for cluster-scoped resources', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'node-1', namespace: '' })]);
    renderTable(descriptor, null);

    expect(screen.queryByLabelText('Namespace')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Filter by name')).toBeInTheDocument();
  });

  it('leaves debug containers out of the container squares', async () => {
    seed([{ name: 'Containers', render: 'containers' }], true, [
      makeRow({
        uid: 'a',
        name: 'pod-a',
        namespace: 'prod',
        cells: ['1/1'],
        containers: [
          { name: 'app', state: 'running', ready: true, restarts: 0, init: false },
          {
            name: 'spinoza-debug-1',
            state: 'running',
            ready: false,
            restarts: 0,
            init: false,
            ephemeral: true,
          },
        ],
      }),
    ]);
    renderTable(descriptor, null);
    await screen.findByRole('button', { name: 'pod-a' });

    expect(screen.getByTitle('app: running')).toBeInTheDocument();
    expect(screen.queryByTitle(/spinoza-debug-1/)).not.toBeInTheDocument();
  });
});

describe('the selected row', () => {
  it('keeps its highlight under the cursor', async () => {
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, row);
    const button = await screen.findByRole('button', { name: 'pod-a' });
    const tr = button.closest('tr');
    if (!tr) {
      throw new Error('row element not found');
    }

    expect(tr.className).toContain('bg-surface-active');
    expect(tr.className).not.toContain('hover:bg-surface-raised');
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Column, ResourceDescriptor, Row } from '../../src/lib/types';
import ResourceTable from '../../src/components/ResourceTable';
import { useResourcesStore } from '../../src/store/resources';
import { makeColumns, makeDescriptor, makeRow } from '../helpers';
import { readTableState, tableKey } from '../../src/lib/tableState';
import { NO_FILTER, imposeFilter } from '../../src/lib/tableFilter';

const SUB = 's1';
const descriptor = makeDescriptor({ resource: 'pods', kind: 'Pod' });

function drag(grip: HTMLElement, moves: number[], read?: () => string): string[] {
  const seen: string[] = [];
  fireEvent.pointerDown(grip, { clientX: 0, pointerId: 1, button: 0 });
  for (const clientX of moves) {
    fireEvent.pointerMove(grip, { clientX, pointerId: 1 });
    if (read) {
      seen.push(read());
    }
  }
  fireEvent.pointerUp(grip, { clientX: moves[moves.length - 1], pointerId: 1 });
  return seen;
}

function seed(columns: Column[], namespaced: boolean, rows: Row[]): void {
  useResourcesStore.getState().applySnapshot(SUB, columns, namespaced, rows);
}

function renderTable(
  active: ResourceDescriptor | null,
  selected: Row | null,
  onSelect = vi.fn(),
  imposed = NO_FILTER,
) {
  return render(
    <ResourceTable
      active={active}
      subId={SUB}
      selected={selected}
      imposed={imposed}
      onSelect={onSelect}
    />,
  );
}

async function namespaceCell(): Promise<HTMLElement> {
  const cells = await screen.findAllByText('prod');
  const cell = cells.find((one) => one.tagName === 'TD');
  if (cell === undefined) {
    throw new Error('namespace cell not found');
  }
  return cell;
}

function resetStore(): void {
  useResourcesStore.setState({ subs: new Map(), errors: new Map() });
}

function names(): string[] {
  return screen
    .getAllByRole('row')
    .slice(1)
    .map((row) => row.querySelector('button')?.textContent ?? '')
    .filter((name) => name !== '');
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

  it('renders Name, snapshot columns and Age for a cluster-scoped resource', () => {
    seed(makeColumns(['Ready', 'Status']), false, []);
    renderTable(descriptor, null);
    const headers = screen
      .getAllByRole('columnheader')
      .slice(1)
      .map((cell) => cell.textContent);
    expect(headers).toEqual(['Name', 'Ready', 'Status', 'Age']);
  });

  it('inserts a Namespace column for a namespaced resource', () => {
    seed(makeColumns(['Ready']), true, []);
    renderTable(descriptor, null);
    const headers = screen
      .getAllByRole('columnheader')
      .slice(1)
      .map((cell) => cell.textContent);
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
      screen.getByTitle('sidecar: waiting (CrashLoopBackOff), 7 restarts'),
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
    const nameHeader = screen.getAllByRole('columnheader')[1];
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
    expect(cells.item(3).textContent).toBe('');
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
    expect(cells.item(2).textContent).toBe('');
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
    const header = screen.getAllByRole('columnheader')[1];
    expect(header).toHaveAttribute('aria-sort', 'none');
    const headerButton = within(header).getByRole('button', { name: /^Name/ });
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

  it('opens the row from anywhere in it, not only the name', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: [] });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, null, onSelect);

    await user.click(await namespaceCell());

    expect(onSelect).toHaveBeenCalledWith(row);
  });

  it('opens the row from the keyboard', async () => {
    const onSelect = vi.fn();
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: [] });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, null, onSelect);
    const cell = await namespaceCell();

    fireEvent.keyDown(cell, { key: 'Enter' });
    expect(onSelect).toHaveBeenCalledWith(row);

    onSelect.mockClear();
    fireEvent.keyDown(cell, { key: 'a' });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it('leaves the row shut when a control inside it was clicked', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: [] });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, null, onSelect);

    await user.click(await screen.findByRole('checkbox', { name: 'Select pod-a' }));

    expect(onSelect).not.toHaveBeenCalled();
  });

  it('leaves the row shut while text is being selected', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: [] });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, null, onSelect);
    const cell = await namespaceCell();
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => 'prod',
    } as unknown as Selection);

    await user.click(cell);

    expect(onSelect).not.toHaveBeenCalled();
  });

  it('opens the row when nothing is selected at all', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const row = makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod', cells: [] });
    seed(makeColumns([]), true, [row]);
    renderTable(descriptor, null, onSelect);
    const cell = await namespaceCell();
    vi.spyOn(window, 'getSelection').mockReturnValue(null);

    await user.click(cell);

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
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
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

  it('sorts nodes by cpu, heaviest first', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            pods: {},
            nodes: {
              'node-1': { cpuMilli: 500, memoryMi: 1024, cpuPercent: 12, memPercent: 60 },
              'node-2': { cpuMilli: 2500, memoryMi: 512, cpuPercent: 80, memPercent: 20 },
            },
          }),
      }),
    );
    const nodeDescriptor = makeDescriptor({ resource: 'nodes', kind: 'Node', namespaced: false });
    seed(makeColumns([]), false, [
      makeRow({ uid: 'a', name: 'node-1', namespace: '' }),
      makeRow({ uid: 'b', name: 'node-2', namespace: '' }),
    ]);
    renderTable(nodeDescriptor, null);
    await screen.findByText('12%');

    await user.click(screen.getByRole('button', { name: /^CPU/ }));

    expect(names()).toEqual(['node-2', 'node-1']);
  });

  it('sorts nodes by memory on its own column', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            pods: {},
            nodes: {
              'node-1': { cpuMilli: 500, memoryMi: 1024, cpuPercent: 12, memPercent: 60 },
              'node-2': { cpuMilli: 2500, memoryMi: 512, cpuPercent: 80, memPercent: 20 },
            },
          }),
      }),
    );
    const nodeDescriptor = makeDescriptor({ resource: 'nodes', kind: 'Node', namespaced: false });
    seed(makeColumns([]), false, [
      makeRow({ uid: 'a', name: 'node-1', namespace: '' }),
      makeRow({ uid: 'b', name: 'node-2', namespace: '' }),
    ]);
    renderTable(nodeDescriptor, null);
    await screen.findByText('60%');

    await user.click(screen.getByRole('button', { name: /^Memory/ }));

    expect(names()).toEqual(['node-1', 'node-2']);
  });

  it('sorts pods by memory, hungriest first', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () =>
          Promise.resolve({
            pods: {
              'prod/pod-a': { cpuMilli: 900, memoryMi: 64, cpuPercent: 0, memPercent: 0 },
              'prod/pod-b': { cpuMilli: 100, memoryMi: 512, cpuPercent: 0, memPercent: 0 },
            },
            nodes: {},
          }),
      }),
    );
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);
    await screen.findByText('64Mi');

    await user.click(screen.getByRole('button', { name: /^Memory/ }));

    expect(names()).toEqual(['pod-b', 'pod-a']);
  });

  it('leaves a pod the metrics server never mentioned at the bottom', async () => {
    const user = userEvent.setup();
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
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);
    await screen.findByText('150m');

    await user.click(screen.getByRole('button', { name: /^CPU/ }));

    expect(names()).toEqual(['pod-a', 'pod-b']);
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

describe('a timestamp column', () => {
  it('reads as an age, and as a dash when the object never reported one', async () => {
    seed([{ name: 'Last seen', render: 'age' }], true, [
      makeRow({
        uid: 'a',
        name: 'web.17abc',
        namespace: 'prod',
        cells: [new Date(Date.now() - 3600000).toISOString()],
      }),
      makeRow({ uid: 'b', name: 'web.17abd', namespace: 'prod', cells: [''] }),
    ]);
    renderTable(descriptor, null);

    await screen.findByRole('button', { name: 'web.17abc' });
    const rows = screen.getAllByRole('row').slice(1);
    expect(rows[0].textContent).toContain('1h');
    expect(rows[1].textContent).toContain('-');
  });
});

describe('a filter imposed from outside', () => {
  it('fills the filter box and narrows the rows', async () => {
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'coredns-1', namespace: 'kube-system' }),
      makeRow({ uid: 'b', name: 'metrics-server', namespace: 'kube-system' }),
    ]);
    renderTable(descriptor, null, vi.fn(), imposeFilter(NO_FILTER, 'coredns'));

    expect(await screen.findByRole('searchbox', { name: 'Filter by name' })).toHaveValue('coredns');
    expect(screen.queryByRole('button', { name: 'metrics-server' })).not.toBeInTheDocument();
  });

  it('lands again when the same text is imposed a second time', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'coredns-1', namespace: 'kube-system' }),
    ]);
    const once = imposeFilter(NO_FILTER, 'coredns');
    const view = renderTable(descriptor, null, vi.fn(), once);
    const box = await screen.findByRole('searchbox', { name: 'Filter by name' });
    await user.clear(box);
    expect(box).toHaveValue('');

    view.rerender(
      <ResourceTable
        active={descriptor}
        subId={SUB}
        selected={null}
        imposed={imposeFilter(once, 'coredns')}
        onSelect={vi.fn()}
      />,
    );

    expect(box).toHaveValue('coredns');
  });

  it('is dropped when the list moves to another resource', async () => {
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'coredns-1', namespace: 'kube-system' }),
    ]);
    const imposed = imposeFilter(NO_FILTER, 'coredns');
    const view = renderTable(descriptor, null, vi.fn(), imposed);
    await screen.findByRole('searchbox', { name: 'Filter by name' });

    useResourcesStore
      .getState()
      .applySnapshot('s2', makeColumns([]), true, [makeRow({ uid: 'c', name: 'node-1' })]);
    view.rerender(
      <ResourceTable
        active={descriptor}
        subId="s2"
        selected={null}
        imposed={imposed}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getByRole('searchbox', { name: 'Filter by name' })).toHaveValue('');
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

describe('selecting several rows at once', () => {
  beforeEach(() => {
    resetStore();
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    resetStore();
  });

  it('offers a checkbox per row and one for the lot', () => {
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);

    expect(screen.getByLabelText('Select every row')).toBeInTheDocument();
    expect(screen.getByLabelText('Select pod-a')).toBeInTheDocument();
    expect(screen.getByLabelText('Select pod-b')).toBeInTheDocument();
  });

  it('counts what is selected and offers a delete', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);

    await user.click(screen.getByLabelText('Select pod-a'));

    expect(screen.getByText('1 Pod selected')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('takes the lot in one click', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [
      makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' }),
      makeRow({ uid: 'b', name: 'pod-b', namespace: 'prod' }),
    ]);
    renderTable(descriptor, null);

    await user.click(screen.getByLabelText('Select every row'));

    expect(screen.getByText('2 Pod objects selected')).toBeInTheDocument();
  });

  it('lets go of the selection again', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    renderTable(descriptor, null);
    await user.click(screen.getByLabelText('Select pod-a'));

    await user.click(screen.getByRole('button', { name: 'Clear selection' }));

    expect(screen.queryByText('1 Pod selected')).not.toBeInTheDocument();
  });

  it('drops the selection when the resource changes', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    const view = renderTable(descriptor, null);
    await user.click(screen.getByLabelText('Select pod-a'));

    useResourcesStore
      .getState()
      .applySnapshot('s2', makeColumns([]), true, [makeRow({ uid: 'c', name: 'node-1' })]);
    view.rerender(
      <ResourceTable
        active={descriptor}
        subId="s2"
        selected={null}
        imposed={NO_FILTER}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.queryByText(/selected/)).not.toBeInTheDocument();
  });
});

describe('choosing which columns to show', () => {
  beforeEach(() => {
    resetStore();
    window.localStorage.clear();
  });

  afterEach(() => {
    resetStore();
  });

  it('hides a column and keeps it hidden across a remount', async () => {
    const user = userEvent.setup();
    seed(makeColumns(['Ready']), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    const first = renderTable(descriptor, null);

    await user.click(screen.getByText('Columns'));
    await user.click(screen.getByRole('checkbox', { name: 'Ready' }));

    expect(
      screen
        .getAllByRole('columnheader')
        .map((cell) => cell.textContent)
        .join(),
    ).not.toContain('Ready');

    first.unmount();
    renderTable(descriptor, null);

    expect(
      screen
        .getAllByRole('columnheader')
        .map((cell) => cell.textContent)
        .join(),
    ).not.toContain('Ready');
  });

  it('never offers to hide the checkbox column', async () => {
    const user = userEvent.setup();
    seed(makeColumns(['Ready']), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);

    await user.click(screen.getByText('Columns'));

    expect(screen.queryByRole('checkbox', { name: 'select' })).not.toBeInTheDocument();
  });
});

describe('a sort the user chose', () => {
  beforeEach(() => {
    resetStore();
    window.localStorage.clear();
  });

  afterEach(() => {
    resetStore();
  });

  it('survives a remount', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), false, [
      makeRow({ uid: 'a', name: 'bravo' }),
      makeRow({ uid: 'b', name: 'alpha' }),
    ]);
    const first = renderTable(descriptor, null);

    await user.click(screen.getAllByRole('columnheader')[1].querySelector('button') as HTMLElement);
    first.unmount();

    renderTable(descriptor, null);

    expect(screen.getAllByRole('columnheader')[1].textContent).toContain('▲');
  });

  it('does not follow the user to another resource kind', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'bravo' })]);
    const view = renderTable(descriptor, null);
    await user.click(screen.getAllByRole('columnheader')[1].querySelector('button') as HTMLElement);

    const other = makeDescriptor({ resource: 'nodes', kind: 'Node', namespaced: false });
    useResourcesStore
      .getState()
      .applySnapshot('s2', makeColumns([]), false, [makeRow({ uid: 'c', name: 'node-1' })]);
    view.rerender(
      <ResourceTable
        active={other}
        subId="s2"
        selected={null}
        imposed={NO_FILTER}
        onSelect={vi.fn()}
      />,
    );

    expect(screen.getAllByRole('columnheader')[1].textContent).not.toContain('▲');
  });
});

describe('a column the user resized', () => {
  beforeEach(() => {
    resetStore();
    window.localStorage.clear();
  });

  afterEach(() => {
    resetStore();
    vi.restoreAllMocks();
  });

  it('keeps its width across a remount', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    const first = renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });

    const before = screen.getAllByRole('columnheader')[1].style.width;
    drag(grip, [60]);
    const widened = screen.getAllByRole('columnheader')[1].style.width;
    expect(widened).not.toBe(before);
    first.unmount();

    renderTable(descriptor, null);

    expect(screen.getAllByRole('columnheader')[1].style.width).toBe(widened);
  });

  it('follows the pointer on the very first move, not one event later', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;

    drag(grip, [60]);

    expect(width()).toBe('300px');
  });

  it('tracks every step of a drag rather than lagging behind it', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;

    const seen = drag(grip, [20, 40, 60], width);

    expect(seen).toEqual(['260px', '280px', '300px']);
    expect(width()).toBe('300px');
  });

  it('drops the flex share once the user sizes that column themselves', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });

    drag(grip, [30]);

    expect(screen.getAllByRole('columnheader')[1].style.width).toBe('270px');
    expect(readTableState(tableKey(descriptor)).sizing).toEqual({ name: 270 });
  });

  it('resizes from the width on screen, not the width behind the flex share', async () => {
    const user = userEvent.setup();
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(2000);
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const width = () => Number.parseInt(screen.getAllByRole('columnheader')[1].style.width, 10);
    const flexed = width();

    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    grip.focus();
    await user.keyboard('{ArrowRight}');

    expect(flexed).toBeGreaterThan(240);
    expect(width()).toBe(flexed + 16);
  });

  it('resizes from the keyboard like the docks do', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;
    const before = width();

    grip.focus();
    await user.keyboard('{ArrowRight}');
    const widened = width();
    await user.keyboard('{ArrowLeft}');

    expect(widened).not.toBe(before);
    expect(width()).toBe(before);
  });

  it('goes back to the default width on Home', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;
    const before = width();

    grip.focus();
    await user.keyboard('{ArrowRight}{ArrowRight}');
    expect(width()).not.toBe(before);
    await user.keyboard('{Home}');

    expect(width()).toBe(before);
  });

  it('will not shrink a column past its minimum', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });

    grip.focus();
    await user.keyboard('{ArrowLeft}'.repeat(20));

    expect(screen.getAllByRole('columnheader')[1].style.width).toBe('100px');
  });
});

describe('copying a row name', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    resetStore();
  });

  it('offers a copy button beside each name', () => {
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'pod-a', namespace: 'prod' })]);
    renderTable(descriptor, null);

    expect(screen.getByRole('button', { name: 'Copy pod-a' })).toBeInTheDocument();
  });
});

describe('the resize grip itself', () => {
  beforeEach(() => {
    resetStore();
    window.localStorage.clear();
  });

  afterEach(() => {
    resetStore();
    vi.restoreAllMocks();
  });

  it('ignores a right-button press', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;
    const before = width();

    fireEvent.pointerDown(grip, { clientX: 0, pointerId: 1, button: 2 });
    fireEvent.pointerMove(grip, { clientX: 60, pointerId: 1 });

    expect(width()).toBe(before);
  });

  it('ignores moves from a pointer that never pressed it', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;

    fireEvent.pointerMove(grip, { clientX: 60, pointerId: 1 });

    expect(width()).toBe('240px');
  });

  it('stops resizing when the pointer capture is taken away', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });
    const width = () => screen.getAllByRole('columnheader')[1].style.width;

    fireEvent.pointerDown(grip, { clientX: 0, pointerId: 1, button: 0 });
    fireEvent.pointerMove(grip, { clientX: 40, pointerId: 1 });
    fireEvent.lostPointerCapture(grip, { pointerId: 1 });
    fireEvent.pointerMove(grip, { clientX: 200, pointerId: 1 });

    expect(width()).toBe('280px');
  });

  it('will not drag a column below its minimum', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });

    drag(grip, [-400]);

    expect(screen.getAllByRole('columnheader')[1].style.width).toBe('100px');
  });
});

describe('pointer capture during a drag', () => {
  beforeEach(() => {
    resetStore();
    window.localStorage.clear();
  });

  afterEach(() => {
    resetStore();
    vi.restoreAllMocks();
  });

  it('captures the pointer so moves keep arriving off the 8px grip', () => {
    const capture = vi.fn();
    const release = vi.fn();
    Object.assign(HTMLElement.prototype, {
      setPointerCapture: capture,
      releasePointerCapture: release,
    });
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });

    drag(grip, [60]);

    expect(capture).toHaveBeenCalledWith(1);
    expect(release).toHaveBeenCalledWith(1);
    expect(screen.getAllByRole('columnheader')[1].style.width).toBe('300px');
  });

  it('ignores a release from a pointer that never pressed it', () => {
    seed(makeColumns([]), false, [makeRow({ uid: 'a', name: 'pod-a' })]);
    renderTable(descriptor, null);
    const grip = screen.getByRole('button', { name: 'Resize the Name column' });

    fireEvent.pointerUp(grip, { clientX: 60, pointerId: 1 });

    expect(screen.getAllByRole('columnheader')[1].style.width).toBe('240px');
  });

  it('clears a name filter that matched nothing', async () => {
    const user = userEvent.setup();
    seed(makeColumns([]), true, [makeRow({ uid: 'a', name: 'web-0', namespace: 'prod' })]);
    renderTable(descriptor, null);
    await user.type(await screen.findByLabelText('Filter by name'), 'nothing-matches');
    expect(screen.getByText('Nothing matches the current filter.')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear filter' }));

    expect(await screen.findByText('web-0')).toBeInTheDocument();
    expect(screen.getByLabelText('Filter by name')).toHaveValue('');
  });
});

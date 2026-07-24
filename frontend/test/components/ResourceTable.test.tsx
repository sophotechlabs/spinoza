import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
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
  useResourcesStore.setState({ subs: new Map() });
}

describe('ResourceTable', () => {
  beforeEach(() => {
    resetStore();
  });

  afterEach(() => {
    vi.useRealTimers();
    resetStore();
  });

  it('renders a placeholder when no resource is active', () => {
    renderTable(null, null);
    expect(screen.getByText('Select a resource to view.')).toBeInTheDocument();
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
    expect(tr.className).toContain('bg-neutral-800');
  });
});

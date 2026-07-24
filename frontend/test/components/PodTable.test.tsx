import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PodTable from '../../src/components/PodTable';
import { makePod } from '../helpers';

afterEach(() => {
  vi.useRealTimers();
});

describe('PodTable', () => {
  it('renders the seven columns in order', () => {
    render(<PodTable rows={[]} selectedUid={null} onSelect={vi.fn()} />);
    const headers = screen.getAllByRole('columnheader').map((cell) => cell.textContent);
    expect(headers).toEqual(['Name', 'Namespace', 'Status', 'Ready', 'Restarts', 'Age', 'Node']);
  });

  it('renders a row for each pod', () => {
    const rows = [makePod({ uid: 'a', name: 'pod-a' }), makePod({ uid: 'b', name: 'pod-b' })];
    render(<PodTable rows={rows} selectedUid={null} onSelect={vi.fn()} />);
    expect(screen.getByRole('button', { name: 'pod-a' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'pod-b' })).toBeInTheDocument();
  });

  it('formats age for seconds, minutes, hours and days', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-24T12:00:00Z'));
    const rows = [
      makePod({ uid: 's', name: 'sec', createdAt: '2026-07-24T11:59:30Z' }),
      makePod({ uid: 'm', name: 'min', createdAt: '2026-07-24T11:55:00Z' }),
      makePod({ uid: 'h', name: 'hour', createdAt: '2026-07-24T09:00:00Z' }),
      makePod({ uid: 'd', name: 'day', createdAt: '2026-07-22T12:00:00Z' }),
    ];
    render(<PodTable rows={rows} selectedUid={null} onSelect={vi.fn()} />);
    expect(screen.getByText('30s')).toBeInTheDocument();
    expect(screen.getByText('5m')).toBeInTheDocument();
    expect(screen.getByText('3h')).toBeInTheDocument();
    expect(screen.getByText('2d')).toBeInTheDocument();
  });

  it('clamps a future createdAt to zero seconds', () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-07-24T12:00:00Z'));
    const rows = [makePod({ uid: 'f', name: 'future', createdAt: '2026-07-24T12:01:40Z' })];
    render(<PodTable rows={rows} selectedUid={null} onSelect={vi.fn()} />);
    expect(screen.getByText('0s')).toBeInTheDocument();
  });

  it('renders an empty age for an unparseable createdAt', () => {
    const rows = [makePod({ uid: 'bad', name: 'bad-pod', createdAt: 'not-a-date' })];
    render(<PodTable rows={rows} selectedUid={null} onSelect={vi.fn()} />);
    const button = screen.getByRole('button', { name: 'bad-pod' });
    const row = button.closest('tr');
    if (!row) {
      throw new Error('row element not found');
    }
    const cells = row.querySelectorAll('td');
    expect(cells.item(5).textContent).toBe('');
  });

  it('toggles sorting through ascending, descending and none', async () => {
    const user = userEvent.setup();
    const rows = [
      makePod({ uid: '1', name: 'gamma', namespace: 'ns' }),
      makePod({ uid: '2', name: 'alpha', namespace: 'ns' }),
    ];
    render(<PodTable rows={rows} selectedUid={null} onSelect={vi.fn()} />);
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

  it('calls onSelect with the pod when its name is clicked', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const pod = makePod({ uid: 'a', name: 'pod-a' });
    render(<PodTable rows={[pod]} selectedUid={null} onSelect={onSelect} />);
    await user.click(screen.getByRole('button', { name: 'pod-a' }));
    expect(onSelect).toHaveBeenCalledWith(pod);
  });

  it('marks the selected row', () => {
    const rows = [makePod({ uid: 'a', name: 'pod-a' })];
    render(<PodTable rows={rows} selectedUid="a" onSelect={vi.fn()} />);
    const row = screen.getByRole('button', { name: 'pod-a' }).closest('tr');
    if (!row) {
      throw new Error('row element not found');
    }
    expect(row.className).toContain('bg-neutral-800');
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category } from '../../src/lib/types';
import Sidebar from '../../src/components/Sidebar';
import { makeCategory, makeDescriptor } from '../helpers';

function stubFetch(categories: Category[]): void {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: () => Promise.resolve(categories),
  });
  vi.stubGlobal('fetch', fetchMock);
}

const categories: Category[] = [
  makeCategory('Workloads', [
    makeDescriptor({ group: '', version: 'v1', resource: 'pods', kind: 'Pod' }),
    makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments', kind: 'Deployment' }),
  ]),
  makeCategory('Config', [
    makeDescriptor({ group: '', version: 'v1', resource: 'configmaps', kind: 'ConfigMap' }),
  ]),
];

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Sidebar', () => {
  it('renders categories with resource counts after discovery loads', async () => {
    stubFetch(categories);
    render(<Sidebar activeResource={null} onSelect={vi.fn()} />);
    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deployment' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'ConfigMap' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Workloads/ })).toHaveTextContent('2');
    expect(screen.getByRole('button', { name: /▾ Config/ })).toHaveTextContent('1');
  });

  it('calls onSelect with the descriptor when a resource is clicked', async () => {
    stubFetch(categories);
    const onSelect = vi.fn();
    render(<Sidebar activeResource={null} onSelect={onSelect} />);
    const button = await screen.findByRole('button', { name: 'Deployment' });
    await userEvent.click(button);
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ group: 'apps', resource: 'deployments' }),
    );
  });

  it('collapses and expands a category', async () => {
    stubFetch(categories);
    render(<Sidebar activeResource={null} onSelect={vi.fn()} />);
    await screen.findByRole('button', { name: 'Pod' });
    const header = screen.getByRole('button', { name: /Workloads/ });
    await userEvent.click(header);
    expect(screen.queryByRole('button', { name: 'Pod' })).not.toBeInTheDocument();
    await userEvent.click(header);
    expect(screen.getByRole('button', { name: 'Pod' })).toBeInTheDocument();
  });

  it('highlights the active resource', async () => {
    stubFetch(categories);
    const active = makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments' });
    render(<Sidebar activeResource={active} onSelect={vi.fn()} />);
    const button = await screen.findByRole('button', { name: 'Deployment' });
    expect(button.className).toContain('bg-neutral-800');
    expect(screen.getByRole('button', { name: 'Pod' }).className).not.toContain('bg-neutral-800');
  });

  it('shows the error message when discovery fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));
    render(<Sidebar activeResource={null} onSelect={vi.fn()} />);
    expect(await screen.findByText('network down')).toBeInTheDocument();
  });

  it('shows a generic message when discovery rejects with a non-error value', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('boom'));
    render(<Sidebar activeResource={null} onSelect={vi.fn()} />);
    expect(await screen.findByText('discovery request failed')).toBeInTheDocument();
  });
});

import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category, ResourceDescriptor, View } from '../../src/lib/types';
import Sidebar from '../../src/components/Sidebar';
import { makeCategory, makeDescriptor } from '../helpers';

interface RenderOverrides {
  view?: View;
  activeResource?: ResourceDescriptor | null;
  onSelect?: (descriptor: ResourceDescriptor) => void;
  onSelectGitops?: () => void;
}

function renderSidebar(overrides: RenderOverrides = {}) {
  const props = {
    view: overrides.view ?? 'resources',
    activeResource: overrides.activeResource ?? null,
    onSelect: overrides.onSelect ?? vi.fn(),
    onSelectGitops: overrides.onSelectGitops ?? vi.fn(),
  };
  return render(<Sidebar {...props} />);
}

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
    renderSidebar();
    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deployment' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'ConfigMap' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Workloads/ })).toHaveTextContent('2');
    expect(screen.getByRole('button', { name: /▾ Config/ })).toHaveTextContent('1');
  });

  it('calls onSelect with the descriptor when a resource is clicked', async () => {
    stubFetch(categories);
    const onSelect = vi.fn();
    renderSidebar({ onSelect });
    const button = await screen.findByRole('button', { name: 'Deployment' });
    await userEvent.click(button);
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ group: 'apps', resource: 'deployments' }),
    );
  });

  it('collapses and expands a category', async () => {
    stubFetch(categories);
    renderSidebar();
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
    renderSidebar({ activeResource: active });
    const button = await screen.findByRole('button', { name: 'Deployment' });
    expect(button.className).toContain('bg-neutral-800');
    expect(screen.getByRole('button', { name: 'Pod' }).className).not.toContain('bg-neutral-800');
  });

  it('shows the error message when discovery fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('network down')));
    renderSidebar();
    expect(await screen.findByText('network down')).toBeInTheDocument();
  });

  it('shows a generic message when discovery rejects with a non-error value', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('boom'));
    renderSidebar();
    expect(await screen.findByText('discovery request failed')).toBeInTheDocument();
  });

  it('calls onSelectGitops when the GitOps entry is clicked', async () => {
    stubFetch(categories);
    const onSelectGitops = vi.fn();
    renderSidebar({ onSelectGitops });
    await userEvent.click(screen.getByRole('button', { name: 'GitOps' }));
    expect(onSelectGitops).toHaveBeenCalledTimes(1);
  });

  it('highlights the GitOps entry when the gitops view is active', () => {
    stubFetch(categories);
    renderSidebar({ view: 'gitops' });
    expect(screen.getByRole('button', { name: 'GitOps' }).className).toContain('bg-neutral-800');
  });

  it('does not highlight the GitOps entry in the resources view', () => {
    stubFetch(categories);
    renderSidebar({ view: 'resources' });
    expect(screen.getByRole('button', { name: 'GitOps' }).className).not.toContain(
      'bg-neutral-800',
    );
  });
});

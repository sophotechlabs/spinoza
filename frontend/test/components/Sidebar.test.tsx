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
  onSelectFlux?: () => void;
  onSelectTiles?: () => void;
  onSelectResources?: () => void;
}

function renderSidebar(overrides: RenderOverrides = {}) {
  const props = {
    view: overrides.view ?? 'resources',
    activeResource: overrides.activeResource ?? null,
    onSelect: overrides.onSelect ?? vi.fn(),
    onSelectGitops: overrides.onSelectGitops ?? vi.fn(),
    onSelectFlux: overrides.onSelectFlux ?? vi.fn(),
    onSelectTiles: overrides.onSelectTiles ?? vi.fn(),
    onSelectResources: overrides.onSelectResources ?? vi.fn(),
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
  it('renders the GitOps section with all four entries', () => {
    stubFetch(categories);
    renderSidebar();
    expect(screen.getByRole('button', { name: 'Graph' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Flux' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Flux Dashboard' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Overview' })).toBeInTheDocument();
  });

  it('starts with resource categories collapsed and expands one on click', async () => {
    stubFetch(categories);
    renderSidebar();
    const header = await screen.findByRole('button', { name: /Workloads/ });
    expect(screen.queryByRole('button', { name: 'Pod' })).not.toBeInTheDocument();
    await userEvent.click(header);
    expect(screen.getByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deployment' })).toBeInTheDocument();
  });

  it('collapses an expanded category again', async () => {
    stubFetch(categories);
    renderSidebar();
    const header = await screen.findByRole('button', { name: /Workloads/ });
    await userEvent.click(header);
    expect(screen.getByRole('button', { name: 'Pod' })).toBeInTheDocument();
    await userEvent.click(header);
    expect(screen.queryByRole('button', { name: 'Pod' })).not.toBeInTheDocument();
  });

  it('shows the resource count on a category header', async () => {
    stubFetch(categories);
    renderSidebar();
    expect(await screen.findByRole('button', { name: /Workloads/ })).toHaveTextContent('2');
  });

  it('calls onSelect with the descriptor when a resource is clicked', async () => {
    stubFetch(categories);
    const onSelect = vi.fn();
    renderSidebar({ onSelect });
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Deployment' }));
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ group: 'apps', resource: 'deployments' }),
    );
  });

  it('highlights the active resource', async () => {
    stubFetch(categories);
    const active = makeDescriptor({ group: 'apps', version: 'v1', resource: 'deployments' });
    renderSidebar({ activeResource: active });
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));
    expect(screen.getByRole('button', { name: 'Deployment' }).className).toContain(
      'bg-neutral-800',
    );
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

  it('calls each GitOps entry handler when clicked', async () => {
    stubFetch(categories);
    const onSelectGitops = vi.fn();
    const onSelectFlux = vi.fn();
    const onSelectTiles = vi.fn();
    const onSelectResources = vi.fn();
    renderSidebar({ onSelectGitops, onSelectFlux, onSelectTiles, onSelectResources });
    await userEvent.click(screen.getByRole('button', { name: 'Graph' }));
    await userEvent.click(screen.getByRole('button', { name: 'Flux' }));
    await userEvent.click(screen.getByRole('button', { name: 'Flux Dashboard' }));
    await userEvent.click(screen.getByRole('button', { name: 'Overview' }));
    expect(onSelectGitops).toHaveBeenCalledTimes(1);
    expect(onSelectFlux).toHaveBeenCalledTimes(1);
    expect(onSelectTiles).toHaveBeenCalledTimes(1);
    expect(onSelectResources).toHaveBeenCalledTimes(1);
  });

  it('highlights the GitOps entry that matches the active view', () => {
    stubFetch(categories);
    renderSidebar({ view: 'flux-tiles' });
    expect(screen.getByRole('button', { name: 'Flux Dashboard' }).className).toContain(
      'bg-neutral-800',
    );
    expect(screen.getByRole('button', { name: 'Graph' }).className).not.toContain('bg-neutral-800');
  });

  it('collapses the GitOps section when its header is clicked', async () => {
    stubFetch(categories);
    renderSidebar();
    expect(screen.getByRole('button', { name: 'Graph' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /GitOps/ }));
    expect(screen.queryByRole('button', { name: 'Graph' })).not.toBeInTheDocument();
  });
  it('nests custom resources under their api group', async () => {
    const user = userEvent.setup();
    stubFetch([
      makeCategory('Custom Resources', [
        makeDescriptor({ group: 'cilium.io', resource: 'ciliumendpoints', kind: 'CiliumEndpoint' }),
        makeDescriptor({ group: 'cilium.io', resource: 'ciliumnodes', kind: 'CiliumNode' }),
        makeDescriptor({ group: 'traefik.io', resource: 'middlewares', kind: 'Middleware' }),
      ]),
    ]);
    renderSidebar();

    await user.click(await screen.findByRole('button', { name: /Custom Resources/ }));

    const cilium = screen.getByRole('button', { name: /cilium\.io/ });
    const traefik = screen.getByRole('button', { name: /traefik\.io/ });
    expect(cilium).toBeInTheDocument();
    expect(traefik).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'CiliumEndpoint' })).not.toBeInTheDocument();

    await user.click(cilium);

    expect(screen.getByRole('button', { name: 'CiliumEndpoint' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'CiliumNode' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Middleware' })).not.toBeInTheDocument();
  });

  it('selects a resource from a nested api group', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    const descriptor = makeDescriptor({
      group: 'cilium.io',
      resource: 'ciliumendpoints',
      kind: 'CiliumEndpoint',
    });
    stubFetch([makeCategory('Custom Resources', [descriptor])]);
    renderSidebar({ onSelect });

    await user.click(await screen.findByRole('button', { name: /Custom Resources/ }));
    await user.click(screen.getByRole('button', { name: /cilium\.io/ }));
    await user.click(screen.getByRole('button', { name: 'CiliumEndpoint' }));

    expect(onSelect).toHaveBeenCalledWith(descriptor);
  });

  it('keeps flat categories flat', async () => {
    const user = userEvent.setup();
    stubFetch([makeCategory('Workloads', [makeDescriptor({ resource: 'pods', kind: 'Pod' })])]);
    renderSidebar();

    await user.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(screen.getByRole('button', { name: 'Pod' })).toBeInTheDocument();
  });
});

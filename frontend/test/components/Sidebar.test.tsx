import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category, ResourceDescriptor, View } from '../../src/lib/types';
import Sidebar from '../../src/components/Sidebar';
import { bumpClusterEpoch } from '../../src/store/cluster';
import { clearCatalog, useCatalogStore } from '../../src/store/catalog';
import { ARGO_VIEWS, FLUX_VIEWS } from '../../src/lib/types';
import { anySignal, makeCategory, makeDescriptor, rejectsWith } from '../helpers';
import { activeClusterNow, useClustersStore } from '../../src/store/clusters';
import { MK1, showing } from '../helpers-clusters';
import { VIEW_LABELS } from '../../src/lib/views';

interface RenderOverrides {
  view?: View;
  activeResource?: ResourceDescriptor | null;
  onSelect?: (descriptor: ResourceDescriptor) => void;
  onSelectView?: (view: View) => void;
}

function sidebarProps(overrides: RenderOverrides = {}) {
  const view: View = overrides.view ?? 'resources';
  return {
    view,
    activeResource: overrides.activeResource ?? null,
    onSelect: overrides.onSelect ?? vi.fn(),
    onSelectView: overrides.onSelectView ?? vi.fn(),
  };
}

function renderSidebar(overrides: RenderOverrides = {}) {
  return render(<Sidebar {...sidebarProps(overrides)} />);
}

function stubCatalog(categories: Category[], error?: string): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ categories, error }) }),
  );
}

function stubFetch(
  categories: Category[],
  counts: Record<string, number> = {},
  failing?: Record<string, number>,
  byPhase?: string[],
  capped?: string[],
): void {
  const fetchMock = vi.fn().mockImplementation((url: string) => {
    if (url.startsWith('/api/resources/counts')) {
      return Promise.resolve({
        ok: true,
        json: () => Promise.resolve({ counts, failing, byPhase, capped }),
      });
    }
    return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
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

const withFlux: Category[] = [
  ...categories,
  makeCategory('Custom resources', [
    makeDescriptor({
      group: 'kustomize.toolkit.fluxcd.io',
      version: 'v1',
      resource: 'kustomizations',
      kind: 'Kustomization',
    }),
  ]),
];

const withArgo: Category[] = [
  ...categories,
  makeCategory('Custom resources', [
    makeDescriptor({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      kind: 'Application',
    }),
    makeDescriptor({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'appprojects',
      kind: 'AppProject',
    }),
  ]),
];

afterEach(() => {
  vi.unstubAllGlobals();
  clearCatalog();
});

describe('Sidebar', () => {
  it('hands the discovered catalog to the rest of the app', async () => {
    stubFetch(categories);
    renderSidebar();

    await screen.findByRole('button', { name: /Workloads/ });

    expect(categoriesNow()).toEqual(categories);
  });

  it('offers helm releases at the top', () => {
    stubFetch(categories);
    renderSidebar();

    expect(screen.getByRole('button', { name: 'Helm releases' })).toBeInTheDocument();
  });

  it('opens the overview from just above the cluster group', async () => {
    const seen: View[] = [];
    stubFetch([makeCategory('Cluster', [makeDescriptor({ resource: 'nodes', kind: 'Node' })])]);
    renderSidebar({
      onSelectView: (view) => {
        seen.push(view);
      },
    });

    const overview = await screen.findByRole('button', { name: 'Cluster Overview' });
    const group = screen.getByRole('button', { name: /^Cluster\s*1$/ });
    await userEvent.click(overview);

    expect(group.className).toContain('uppercase');
    expect(overview.className).not.toContain('uppercase');
    expect(seen).toEqual(['cluster']);
  });

  it('keeps the overview on screen while the group is collapsed', async () => {
    stubFetch([makeCategory('Cluster', [makeDescriptor({ resource: 'nodes', kind: 'Node' })])]);
    renderSidebar();
    const group = await screen.findByRole('button', { name: /^Cluster\s*1$/ });
    await userEvent.click(group);
    expect(screen.getByRole('button', { name: /^Node/ })).toBeInTheDocument();

    await userEvent.click(group);

    expect(screen.queryByRole('button', { name: /^Node/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cluster Overview' })).toBeInTheDocument();
  });

  it('keeps the overview at the top when the cluster has no such group', async () => {
    const seen: View[] = [];
    stubFetch(categories);
    renderSidebar({
      onSelectView: (view) => {
        seen.push(view);
      },
    });

    const overview = await screen.findByRole('button', { name: 'Cluster Overview' });
    await userEvent.click(overview);

    expect(overview.parentElement).toHaveAttribute('aria-label', 'Cluster views');
    expect(seen).toEqual(['cluster']);
  });

  it('keeps the overview reachable when discovery fails', async () => {
    stubCatalog([], 'discovery is not available');
    renderSidebar();

    expect(await screen.findByRole('button', { name: 'Cluster Overview' })).toBeInTheDocument();
  });

  it('marks the open top view for assistive technology', async () => {
    stubFetch(categories);
    renderSidebar({ view: 'helm' });

    expect(screen.getByRole('button', { name: 'Helm releases' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(await screen.findByRole('button', { name: 'Cluster Overview' })).not.toHaveAttribute(
      'aria-current',
    );
  });

  it('offers one row per view in each gitops group', async () => {
    const customResources = makeCategory('Custom resources', [
      ...withFlux[withFlux.length - 1].resources,
      ...withArgo[withArgo.length - 1].resources,
    ]);
    stubFetch([...categories, customResources]);
    renderSidebar();
    await screen.findByRole('button', { name: 'Flux Graph' });

    const flux = screen.getAllByRole('button', { name: /^Flux / });
    const argo = screen.getAllByRole('button', { name: /^Argo CD / });

    expect(flux).toHaveLength(FLUX_VIEWS.length);
    expect(argo).toHaveLength(ARGO_VIEWS.length);
    for (const view of [...FLUX_VIEWS, ...ARGO_VIEWS]) {
      expect(screen.getByRole('button', { name: VIEW_LABELS[view] })).toBeInTheDocument();
    }
  });

  it('renders the Flux section once Flux is found in the cluster', async () => {
    stubFetch(withFlux);
    renderSidebar();
    expect(await screen.findByRole('button', { name: 'Flux Graph' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Flux Resource list' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Flux Overview' })).toBeInTheDocument();
  });

  it('says why the Flux section is shut on a cluster without it', async () => {
    stubFetch(categories);
    renderSidebar();

    const header = await screen.findByRole('button', { name: 'Flux' });
    expect(header).toBeDisabled();
    expect(header).toHaveAttribute('title', 'Flux is not found in this cluster');
    expect(screen.queryByRole('button', { name: 'Flux Graph' })).not.toBeInTheDocument();
  });

  it('says why the Argo CD section is shut on a cluster without it', async () => {
    stubFetch(categories);
    renderSidebar();

    const header = await screen.findByRole('button', { name: 'Argo CD' });
    expect(header).toBeDisabled();
    expect(header).toHaveAttribute('title', 'Argo CD is not found in this cluster');
  });

  it('drops the missing-engine tooltip once Argo CD is found', async () => {
    stubFetch(withArgo);
    renderSidebar();

    const header = await screen.findByRole('button', { name: 'Argo CD' });

    expect(header).toBeEnabled();
    expect(header.hasAttribute('title')).toBe(false);
  });

  it('gives Argo CD the same three views Flux has', async () => {
    stubFetch(withArgo);
    renderSidebar();

    expect(await screen.findByRole('button', { name: 'Argo CD Overview' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Argo CD Graph' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Argo CD Resource list' })).toBeInTheDocument();
  });

  it('leaves the Argo CD kinds to the resource categories', async () => {
    stubFetch(withArgo);
    renderSidebar();
    await screen.findByRole('button', { name: 'Argo CD Overview' });

    expect(screen.queryByRole('button', { name: 'AppProject' })).not.toBeInTheDocument();
  });

  it('collapses the Argo CD section when its header is clicked', async () => {
    stubFetch(withArgo);
    renderSidebar();
    expect(await screen.findByRole('button', { name: 'Argo CD Overview' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Argo CD' }));

    expect(screen.queryByRole('button', { name: 'Argo CD Overview' })).not.toBeInTheDocument();
  });

  it('opens an Argo CD view from its entry', async () => {
    const seen: string[] = [];
    stubFetch(withArgo);
    renderSidebar({
      onSelectView: (view) => {
        seen.push(view);
      },
    });

    await userEvent.click(await screen.findByRole('button', { name: 'Argo CD Graph' }));

    expect(seen).toEqual(['argo-graph']);
  });

  it('tells two kinds with the same name apart by their api group', async () => {
    stubFetch([
      makeCategory('Cluster', [
        makeDescriptor({ group: '', version: 'v1', resource: 'events', kind: 'Event' }),
        makeDescriptor({
          group: 'events.k8s.io',
          version: 'v1',
          resource: 'events',
          kind: 'Event',
        }),
        makeDescriptor({ group: '', version: 'v1', resource: 'nodes', kind: 'Node' }),
      ]),
    ]);
    renderSidebar();

    await userEvent.click(await screen.findByRole('button', { name: /^Cluster\s*3$/ }));

    expect(screen.getByRole('button', { name: /Event \(core\)/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Event \(events.k8s.io\)/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Node/ })).toBeInTheDocument();
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
      'bg-surface-active',
    );
    expect(screen.getByRole('button', { name: 'Pod' }).className).not.toContain(
      'bg-surface-active',
    );
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

  it('names the view behind every Flux entry', async () => {
    stubFetch(withFlux);
    const onSelectView = vi.fn<(view: View) => void>();
    renderSidebar({ onSelectView });
    await userEvent.click(await screen.findByRole('button', { name: 'Flux Graph' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Flux Resource list' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Flux Overview' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Flux Overview' }));
    const seen = onSelectView.mock.calls.map((call) => call[0]);
    expect(seen).toEqual(['gitops', 'flux-list', 'flux-roles', 'flux-roles']);
  });

  it('highlights the Flux entry that matches the active view', async () => {
    stubFetch(withFlux);
    renderSidebar({ view: 'flux-roles' });
    expect((await screen.findByRole('button', { name: 'Flux Overview' })).className).toContain(
      'bg-surface-active',
    );
    expect(screen.getByRole('button', { name: 'Flux Graph' }).className).not.toContain(
      'bg-surface-active',
    );
  });

  it('collapses the Flux section when its header is clicked', async () => {
    stubFetch(withFlux);
    renderSidebar();
    expect(await screen.findByRole('button', { name: 'Flux Graph' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Flux' }));
    expect(screen.queryByRole('button', { name: 'Flux Graph' })).not.toBeInTheDocument();
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

  it('resizes from the handle', () => {
    stubFetch(categories);
    renderSidebar({});
    const handle = screen.getByRole('button', { name: 'Resize sidebar' });
    const panel = handle.parentElement;

    fireEvent.mouseDown(handle, { clientX: 224 });
    fireEvent.mouseMove(window, { clientX: 324, buttons: 1 });
    fireEvent.mouseUp(window);

    expect(panel).toHaveStyle({ width: '324px' });
  });

  it('nudges the width from the keyboard', () => {
    stubFetch(categories);
    renderSidebar({});
    const handle = screen.getByRole('button', { name: 'Resize sidebar' });
    const panel = handle.parentElement;

    fireEvent.keyDown(handle, { key: 'ArrowRight' });
    expect(panel).toHaveStyle({ width: '256px' });

    fireEvent.keyDown(handle, { key: 'ArrowLeft' });
    expect(panel).toHaveStyle({ width: '224px' });

    fireEvent.keyDown(handle, { key: 'Enter' });
    expect(panel).toHaveStyle({ width: '224px' });
  });

  it('surfaces a discovery failure with a retry', async () => {
    stubCatalog([], 'the server could not find the requested resource');
    renderSidebar({});

    expect(await screen.findByText('Discovery failed')).toBeInTheDocument();
    expect(
      screen.getByText('the server could not find the requested resource'),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.queryByText('No resource types discovered.')).not.toBeInTheDocument();
  });

  it('re-runs discovery from the retry button', async () => {
    const user = userEvent.setup();
    stubCatalog([], 'connection refused');
    renderSidebar({});
    await screen.findByText('Discovery failed');

    stubCatalog(categories);
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(await screen.findByText(/Workloads/)).toBeInTheDocument();
    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();
    expect(categoriesNow()).toEqual(categories);
    const call = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call).toEqual(['/api/resources', { method: 'POST', signal: anySignal() }]);
  });

  it('reports a failed retry', async () => {
    const user = userEvent.setup();
    stubCatalog([], 'connection refused');
    renderSidebar({});
    await screen.findByText('Discovery failed');

    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 503, json: () => Promise.resolve({}) }),
    );
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(await screen.findByText('discovery request failed with status 503')).toBeInTheDocument();
  });

  it('does not apply a manual retry after the cluster changes', async () => {
    const user = userEvent.setup();
    stubCatalog([], 'connection refused');
    renderSidebar({});
    await screen.findByText('Discovery failed');
    let finishRetry: (response: unknown) => void = () => undefined;
    const pendingRetry = new Promise((resolve) => {
      finishRetry = resolve;
    });
    const oldCategories = [
      makeCategory('Old cluster', [
        makeDescriptor({ group: '', version: 'v1', resource: 'pods', kind: 'Pod' }),
      ]),
    ];
    const newCategories = [
      makeCategory('New cluster', [
        makeDescriptor({ group: '', version: 'v1', resource: 'configmaps', kind: 'ConfigMap' }),
      ]),
    ];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.startsWith('/api/resources/counts')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve({ counts: {} }) });
        }
        if (init?.method === 'POST') {
          return pendingRetry;
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ categories: newCategories }),
        });
      }),
    );
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    act(() => {
      bumpClusterEpoch();
    });

    expect(await screen.findByRole('button', { name: /New cluster/ })).toBeInTheDocument();
    await act(async () => {
      finishRetry({
        ok: true,
        json: () => Promise.resolve({ categories: oldCategories }),
      });
      await pendingRetry;
    });

    expect(screen.queryByRole('button', { name: /Old cluster/ })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /New cluster/ })).toBeInTheDocument();
  });

  it('drops a catalog that lands after unmount', async () => {
    const deferred = {
      settle: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            deferred.settle = () => {
              resolve({
                ok: true,
                json: () => Promise.resolve({ categories, error: 'too late' }),
              });
            };
          }),
      ),
    );
    const view = renderSidebar({});

    view.unmount();
    deferred.settle();
    await Promise.resolve();

    expect(screen.queryByText('too late')).not.toBeInTheDocument();
  });

  it('says so when discovery succeeded but found nothing', async () => {
    stubCatalog([]);
    renderSidebar({});

    expect(await screen.findByText('No resource types discovered.')).toBeInTheDocument();
    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();
  });
});

describe('the active sidebar entry', () => {
  it('keeps its highlight under the cursor', async () => {
    stubFetch(withFlux);
    renderSidebar({ view: 'flux-roles' });
    const active = await screen.findByRole('button', { name: 'Flux Overview' });

    expect(active.className).toContain('bg-surface-active');
    expect(active.className).not.toContain('hover:bg-surface-raised');
  });

  it('does not carry a text colour that a later rule overrides', async () => {
    stubFetch(withFlux);
    renderSidebar({ view: 'flux-roles' });
    const active = await screen.findByRole('button', { name: 'Flux Overview' });

    expect(active.className).toContain('text-fg-strong');
    expect(active.className).not.toContain('text-fg-soft');
  });
});

describe('resource counts', () => {
  it('shows how many objects each type holds', async () => {
    stubFetch(categories, { '/v1/pods': 57, 'apps/v1/deployments': 0 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(await screen.findByRole('button', { name: 'Pod 57' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Deployment 0' })).toBeInTheDocument();
  });

  it('dims a type that holds nothing', async () => {
    stubFetch(categories, { '/v1/pods': 57, 'apps/v1/deployments': 0 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));
    const empty = await screen.findByRole('button', { name: 'Deployment 0' });

    expect(empty.className).toContain('text-fg-subtle');
    expect(screen.getByRole('button', { name: 'Pod 57' }).className).toContain('text-fg-soft');
  });

  it('sinks the empty types below the ones with objects', async () => {
    stubFetch(categories, { '/v1/pods': 0, 'apps/v1/deployments': 3 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));
    await screen.findByRole('button', { name: 'Pod 0' });

    const labels = screen
      .getAllByRole('button')
      .map((node) => node.textContent)
      .filter((text) => text.startsWith('Pod') || text.startsWith('Deployment'));
    expect(labels[0]).toContain('Deployment');
    expect(labels[1]).toContain('Pod');
  });

  it('marks a type it was not allowed to count', async () => {
    stubFetch(categories, { '/v1/pods': -1 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(await screen.findByRole('button', { name: 'Pod -' })).toBeInTheDocument();
  });

  it('carries on when the counts request fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (url.startsWith('/api/resources/counts')) {
          return Promise.reject(new Error('counts down'));
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
      }),
    );
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.getByText(/Object counts unavailable/)).toBeInTheDocument();
    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();
  });

  it('names a counts refusal without a message', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (url.startsWith('/api/resources/counts')) {
          return rejectsWith('nope')();
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
      }),
    );
    renderSidebar();

    expect(
      await screen.findByText(/Object counts unavailable: resource counts request failed/),
    ).toBeInTheDocument();
  });

  it('drops the previous cluster counts when the context changes', async () => {
    stubFetch(categories, { '/v1/pods': 57 });
    const view = renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));
    await screen.findByRole('button', { name: 'Pod 57' });

    const pending = new Promise(() => undefined);
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (url.startsWith('/api/resources/counts')) {
          return pending;
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
      }),
    );
    act(() => {
      bumpClusterEpoch();
    });
    view.rerender(<Sidebar {...sidebarProps()} />);

    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Pod 57' })).not.toBeInTheDocument();
  });

  it('drops the previous cluster catalog error while the next cluster loads', async () => {
    stubCatalog(categories, 'old cluster refused discovery');
    const view = renderSidebar();
    await screen.findByText('old cluster refused discovery');
    expect(screen.getByRole('button', { name: /Workloads/ })).toBeInTheDocument();

    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(() => new Promise(() => undefined)),
    );
    act(() => {
      bumpClusterEpoch();
    });
    view.rerender(<Sidebar {...sidebarProps()} />);

    expect(screen.queryByText('old cluster refused discovery')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Workloads/ })).not.toBeInTheDocument();
  });

  it('carries on when the counts payload has no counts', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (url.startsWith('/api/resources/counts')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
        }
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });
      }),
    );
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
  });
});

describe('what the sidebar tells assistive technology', () => {
  it('says whether a section is open or shut', async () => {
    const user = userEvent.setup();
    stubFetch(categories, {});
    renderSidebar();
    const toggle = await screen.findByRole('button', { name: /Workloads/ });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');

    await user.click(toggle);

    expect(screen.getByRole('button', { name: /Workloads/ })).toHaveAttribute(
      'aria-expanded',
      'true',
    );
  });

  it('says which view is the current one', async () => {
    stubFetch(withFlux, {});
    renderSidebar({ view: 'gitops' });

    expect(await screen.findByRole('button', { name: 'Flux Graph' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(await screen.findByRole('button', { name: 'Flux Overview' })).not.toHaveAttribute(
      'aria-current',
    );
  });

  it('says which resource is the current one', async () => {
    const user = userEvent.setup();
    stubFetch(categories, {});
    renderSidebar({ activeResource: makeDescriptor({ resource: 'pods', kind: 'Pod' }) });
    await user.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(await screen.findByRole('button', { name: 'Pod' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });

  it('keeps the chevron glyphs out of the accessible name', async () => {
    stubFetch(categories, {});
    renderSidebar();

    const header = await screen.findByRole('button', { name: /Workloads/ });
    expect(header.textContent).toContain('▸');
    expect(header).toHaveAccessibleName('Workloads2');
  });
});

describe('a sidebar that survives a reload', () => {
  it('keeps a category open across a remount', async () => {
    const user = userEvent.setup();
    stubFetch(categories, {});
    const first = renderSidebar();
    await user.click(await screen.findByRole('button', { name: /Workloads/ }));
    expect(screen.getByRole('button', { name: 'Pod' })).toBeInTheDocument();
    first.unmount();

    renderSidebar();

    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
  });

  it('keeps the Flux section shut across a remount', async () => {
    const user = userEvent.setup();
    stubFetch(withFlux, {});
    const first = renderSidebar();
    await user.click(await screen.findByRole('button', { name: 'Flux' }));
    expect(screen.queryByRole('button', { name: 'Flux Graph' })).not.toBeInTheDocument();
    first.unmount();

    renderSidebar();

    expect(screen.queryByRole('button', { name: 'Flux Graph' })).not.toBeInTheDocument();
  });

  it('leaves a category the user never touched shut', async () => {
    const user = userEvent.setup();
    stubFetch(categories, {});
    const first = renderSidebar();
    await user.click(await screen.findByRole('button', { name: /Workloads/ }));
    first.unmount();

    renderSidebar();

    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'ConfigMap' })).not.toBeInTheDocument();
  });

  it('keeps a nested api group open across a remount', async () => {
    const user = userEvent.setup();
    stubFetch(
      [
        makeCategory('Custom Resources', [
          makeDescriptor({ group: 'cilium.io', resource: 'ciliumnodes', kind: 'CiliumNode' }),
        ]),
      ],
      {},
    );
    const first = renderSidebar();
    await user.click(await screen.findByRole('button', { name: /Custom Resources/ }));
    await user.click(screen.getByRole('button', { name: /cilium\.io/ }));
    expect(screen.getByRole('button', { name: 'CiliumNode' })).toBeInTheDocument();
    first.unmount();

    renderSidebar();

    expect(await screen.findByRole('button', { name: 'CiliumNode' })).toBeInTheDocument();
  });
});

describe('pods that are not running', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps the kind plain and shows the failing count in a red badge', async () => {
    stubFetch(categories, { '/v1/pods': 12, 'apps/v1/deployments': 4 }, { '/v1/pods': 3 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods.querySelector('span')?.className).not.toContain('text-error');
    expect(pods.querySelector('.text-error')?.textContent).toBe('(3)');
    expect(pods).toHaveAttribute('title', 'Pod: 3 of 12 not ready');
    expect(pods).toHaveTextContent('3 not ready');
  });

  it('marks a tally that stopped at the ceiling rather than reading it as exact', async () => {
    stubFetch(
      categories,
      { '/v1/pods': 42000, 'apps/v1/deployments': 4 },
      { '/v1/pods': 10000 },
      ['/v1/pods'],
      ['/v1/pods'],
    );
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods.querySelector('.text-error')?.textContent).toBe('(10000+)');
    expect(pods).toHaveAttribute('title', 'Pod: 10000+ of 42000 not Running or Succeeded');
    expect(pods).toHaveTextContent('10000+ not Running or Succeeded');
  });

  it('leaves a completed tally unmarked', async () => {
    stubFetch(categories, { '/v1/pods': 12 }, { '/v1/pods': 3 }, [], []);
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods.querySelector('.text-error')?.textContent).toBe('(3)');
  });

  it('explains the failing count without a total when the tally is missing', async () => {
    stubFetch(categories, { 'apps/v1/deployments': 4 }, { '/v1/pods': 3 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods).toHaveAttribute('title', 'Pod: 3 not ready');
    expect(pods.querySelector('.text-error')?.textContent).toBe('(3)');
  });

  it('says what a phase-counted tally actually counted, so a crashloop is not implied', async () => {
    stubFetch(categories, { '/v1/pods': 12, 'apps/v1/deployments': 4 }, { '/v1/pods': 3 }, [
      '/v1/pods',
    ]);
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods).toHaveAttribute('title', 'Pod: 3 of 12 not Running or Succeeded');
    expect(pods).toHaveTextContent('3 not Running or Succeeded');
  });

  it('keeps saying not ready for a kind counted off a warm cache', async () => {
    stubFetch(
      categories,
      { '/v1/pods': 12, 'apps/v1/deployments': 4 },
      { 'apps/v1/deployments': 2 },
      ['/v1/pods'],
    );
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const deployments = await screen.findByRole('button', { name: /^Deployment/ });
    expect(deployments).toHaveAttribute('title', 'Deployment: 2 of 4 not ready');
  });

  it('leaves a healthy kind alone', async () => {
    stubFetch(categories, { '/v1/pods': 12, 'apps/v1/deployments': 4 }, { '/v1/pods': 3 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const deployments = await screen.findByRole('button', { name: /^Deployment/ });
    expect(deployments.querySelector('span')?.className).not.toContain('text-error');
    expect(deployments).toHaveAttribute('title', 'Deployment');
  });

  it('says nothing when the server reports no failures at all', async () => {
    stubFetch(categories, { '/v1/pods': 12 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods.querySelector('span')?.className).not.toContain('text-error');
    expect(pods.querySelector('.text-error')).toBeNull();
  });
});

describe('count alignment', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('gives the section tally and the resource counts the same right edge', async () => {
    stubFetch(categories, { '/v1/pods': 12 });
    renderSidebar();
    const section = await screen.findByRole('button', { name: /Workloads/ });
    await userEvent.click(section);

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(section.className).toContain('px-3');
    expect(pods.className).toContain('pr-3');
    expect(pods.className).not.toContain('px-6');
  });

  it('keeps the total flush right when the failing badge appears', async () => {
    stubFetch(categories, { '/v1/pods': 12 }, { '/v1/pods': 3 });
    renderSidebar();
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    const pods = await screen.findByRole('button', { name: /^Pod/ });
    expect(pods.className).toContain('pr-3');
    const text = pods.textContent;
    expect(text.indexOf('(3)')).toBeGreaterThan(-1);
    expect(text.indexOf('(3)')).toBeLessThan(text.indexOf('12'));
  });
});

describe('discovery retries itself', () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  function stubSequence(replies: (() => unknown)[]): () => number {
    let call = 0;
    let discoveries = 0;
    const fetchMock = vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/resources/counts')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ counts: {} }) });
      }
      if (!url.startsWith('/api/resources')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({}) });
      }
      discoveries += 1;
      const reply = replies[Math.min(call, replies.length - 1)];
      call += 1;
      return reply();
    });
    vi.stubGlobal('fetch', fetchMock);
    return () => discoveries;
  }

  const refused = () => Promise.reject(new Error('discovery request failed with status 503'));
  const partial = () =>
    Promise.resolve({ ok: true, json: () => Promise.resolve({ categories: [], error: 'stale' }) });
  const good = () => Promise.resolve({ ok: true, json: () => Promise.resolve({ categories }) });

  it('clears a transient discovery failure without anyone clicking Retry', async () => {
    vi.useFakeTimers();
    stubSequence([refused, good]);
    renderSidebar({});

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('Discovery failed')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Workloads/ })).toBeInTheDocument();
  });

  it('retries a catalog that comes back carrying an error', async () => {
    vi.useFakeTimers();
    stubSequence([partial, good]);
    renderSidebar({});

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('stale')).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();
  });

  it('backs off and gives up after six attempts, leaving the button', async () => {
    vi.useFakeTimers();
    const discoveries = stubSequence([refused]);
    renderSidebar({});

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(discoveries()).toBe(1);

    for (const delay of [500, 1000, 2000, 4000, 5000, 5000]) {
      await act(async () => {
        await vi.advanceTimersByTimeAsync(delay);
      });
    }
    expect(discoveries()).toBe(7);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60000);
    });
    expect(discoveries()).toBe(7);
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });

  it('does not let a pending automatic retry undo a successful manual one', async () => {
    vi.useFakeTimers();
    stubSequence([refused, refused]);
    renderSidebar({});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.getByText('Discovery failed')).toBeInTheDocument();

    stubSequence([good]);
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(screen.queryByText('Discovery failed')).not.toBeInTheDocument();
  });

  it('drops a scheduled retry when the cluster changes', async () => {
    vi.useFakeTimers();
    const discoveries = stubSequence([refused]);
    const { unmount } = renderSidebar({});
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(discoveries()).toBe(1);

    unmount();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(discoveries()).toBe(1);
  });
});

describe('the fleet entry', () => {
  it('is offered only once a second cluster is open', async () => {
    stubCatalog([]);
    renderSidebar();
    await screen.findByRole('button', { name: 'Issues' });
    expect(screen.queryByRole('button', { name: 'Fleet' })).toBeNull();

    act(() => {
      showing(MK1);
    });

    expect(await screen.findByRole('button', { name: 'Fleet' })).toBeTruthy();
    useClustersStore.getState().reset();
  });

  it('opens the fleet view when it is picked', async () => {
    const user = userEvent.setup();
    const onSelectView = vi.fn();
    stubCatalog([]);
    act(() => {
      showing(MK1);
    });
    renderSidebar({ onSelectView });

    await user.click(await screen.findByRole('button', { name: 'Fleet' }));

    expect(onSelectView).toHaveBeenCalledWith('fleet');
    useClustersStore.getState().reset();
  });
});

function categoriesNow(): Category[] {
  return useCatalogStore.getState().categories[activeClusterNow()] ?? [];
}

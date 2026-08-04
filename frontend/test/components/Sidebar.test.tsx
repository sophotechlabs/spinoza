import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Category, ResourceDescriptor, View } from '../../src/lib/types';
import Sidebar from '../../src/components/Sidebar';
import { anySignal, makeCategory, makeDescriptor, rejectsWith } from '../helpers';

interface RenderOverrides {
  epoch?: number;
  view?: View;
  activeResource?: ResourceDescriptor | null;
  onSelect?: (descriptor: ResourceDescriptor) => void;
  onSelectGraph?: () => void;
  onSelectList?: () => void;
  onSelectOverview?: () => void;
  onSelectRoles?: () => void;
}

function sidebarProps(overrides: RenderOverrides = {}) {
  const view: View = overrides.view ?? 'resources';
  return {
    epoch: overrides.epoch ?? 0,
    view,
    activeResource: overrides.activeResource ?? null,
    onSelect: overrides.onSelect ?? vi.fn(),
    onSelectGraph: overrides.onSelectGraph ?? vi.fn(),
    onSelectList: overrides.onSelectList ?? vi.fn(),
    onSelectOverview: overrides.onSelectOverview ?? vi.fn(),
    onSelectRoles: overrides.onSelectRoles ?? vi.fn(),
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

function stubFetch(categories: Category[], counts: Record<string, number> = {}): void {
  const fetchMock = vi.fn().mockImplementation((url: string) => {
    if (url.startsWith('/api/resources/counts')) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve({ counts }) });
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

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('Sidebar', () => {
  it('renders the GitOps section with all four entries', () => {
    stubFetch(categories);
    renderSidebar();
    expect(screen.getByRole('button', { name: 'Graph' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Resource list' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Overview' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Status tiles' })).toBeInTheDocument();
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

  it('calls each GitOps entry handler when clicked', async () => {
    stubFetch(categories);
    const onSelectGraph = vi.fn();
    const onSelectList = vi.fn();
    const onSelectOverview = vi.fn();
    const onSelectRoles = vi.fn();
    renderSidebar({ onSelectGraph, onSelectList, onSelectOverview, onSelectRoles });
    await userEvent.click(screen.getByRole('button', { name: 'Graph' }));
    await userEvent.click(screen.getByRole('button', { name: 'Resource list' }));
    await userEvent.click(screen.getByRole('button', { name: 'Status tiles' }));
    await userEvent.click(screen.getByRole('button', { name: 'Overview' }));
    expect(onSelectGraph).toHaveBeenCalledTimes(1);
    expect(onSelectList).toHaveBeenCalledTimes(1);
    expect(onSelectOverview).toHaveBeenCalledTimes(1);
    expect(onSelectRoles).toHaveBeenCalledTimes(1);
  });

  it('highlights the GitOps entry that matches the active view', () => {
    stubFetch(categories);
    renderSidebar({ view: 'flux-overview' });
    expect(screen.getByRole('button', { name: 'Status tiles' }).className).toContain(
      'bg-surface-active',
    );
    expect(screen.getByRole('button', { name: 'Graph' }).className).not.toContain(
      'bg-surface-active',
    );
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
  it('keeps its highlight under the cursor', () => {
    stubFetch(categories);
    renderSidebar({ view: 'flux-roles' });
    const active = screen.getByRole('button', { name: 'Overview' });

    expect(active.className).toContain('bg-surface-active');
    expect(active.className).not.toContain('hover:bg-surface-raised');
  });

  it('does not carry a text colour that a later rule overrides', () => {
    stubFetch(categories);
    renderSidebar({ view: 'flux-roles' });
    const active = screen.getByRole('button', { name: 'Overview' });

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

    expect(await screen.findByRole('button', { name: 'Pod —' })).toBeInTheDocument();
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
      await screen.findByText(/Object counts unavailable — resource counts request failed/),
    ).toBeInTheDocument();
  });

  it('drops the previous cluster counts when the context changes', async () => {
    stubFetch(categories, { '/v1/pods': 57 });
    const view = renderSidebar({ epoch: 0 });
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
    view.rerender(<Sidebar {...sidebarProps({ epoch: 1 })} />);
    await userEvent.click(await screen.findByRole('button', { name: /Workloads/ }));

    expect(await screen.findByRole('button', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Pod 57' })).not.toBeInTheDocument();
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

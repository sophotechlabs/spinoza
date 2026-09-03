import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CommandPalette from '../../src/components/CommandPalette';
import { clearRecents, rememberObject } from '../../src/store/recents';
import { bumpClusterEpoch } from '../../src/store/cluster';
import { makeCategory, makeDescriptor } from '../helpers';

const podType = makeDescriptor({ resource: 'pods', kind: 'Pod' });
const deploymentType = makeDescriptor({
  group: 'apps',
  resource: 'deployments',
  kind: 'Deployment',
});

const categories = [
  makeCategory('Workloads', [podType, deploymentType]),
  makeCategory('Custom resources', [
    makeDescriptor({
      group: 'kustomize.toolkit.fluxcd.io',
      version: 'v1',
      resource: 'kustomizations',
      kind: 'Kustomization',
    }),
  ]),
];

function stubCatalog(): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ categories }) }),
  );
}

interface Sweep {
  hits?: unknown[];
  truncated?: boolean;
  errors?: Record<string, string>;
  ok?: boolean;
}

function stubSearch(sweep: Sweep): string[] {
  const asked: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      if (url.startsWith('/api/search')) {
        asked.push(url);
        return Promise.resolve({
          ok: sweep.ok ?? true,
          status: sweep.ok === false ? 500 : 200,
          json: () =>
            Promise.resolve({
              hits: sweep.hits ?? [],
              truncated: sweep.truncated ?? false,
              errors: sweep.errors,
            }),
        });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ categories }),
      });
    }),
  );
  return asked;
}

const clusterHit = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  kind: 'Deployment',
  namespace: 'airbyte',
  name: 'airbyte-server',
};

function renderPalette(open = true) {
  const onClose = vi.fn();
  const onSelectView = vi.fn();
  const onSelectResource = vi.fn();
  const onOpenObject = vi.fn();
  const view = render(
    <CommandPalette
      open={open}
      onClose={onClose}
      onSelectView={onSelectView}
      onSelectResource={onSelectResource}
      onOpenObject={onOpenObject}
    />,
  );
  return { onClose, onSelectView, onSelectResource, onOpenObject, view };
}

beforeEach(() => {
  clearRecents();
  stubCatalog();
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.open = false;
  };
});

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    clearRecents();
  });
});

describe('CommandPalette', () => {
  it('lists the discovered kinds once it opens', async () => {
    renderPalette();

    expect(await screen.findByRole('button', { name: /Pod/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Deployment/ })).toBeInTheDocument();
  });

  it('offers every view the cluster can serve', async () => {
    renderPalette();

    expect(await screen.findByRole('button', { name: /Flux Graph/ })).toBeInTheDocument();
  });

  it('does not fetch the catalog while it is closed', () => {
    renderPalette(false);

    expect(vi.mocked(fetch)).not.toHaveBeenCalled();
  });

  it('narrows the list as you type', async () => {
    const user = userEvent.setup();
    renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.type(screen.getByLabelText(/Search resources/), 'deploy');

    expect(screen.getByRole('button', { name: /Deployment/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Pod/ })).not.toBeInTheDocument();
  });

  it('says so when nothing matches', async () => {
    const user = userEvent.setup();
    const { onSelectView, onSelectResource, onOpenObject } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.type(screen.getByLabelText(/Search resources/), 'zzzz');
    await user.keyboard('{ArrowDown}{Enter}');

    expect(screen.getByText('Nothing matches that.')).toBeInTheDocument();
    expect(onSelectView).not.toHaveBeenCalled();
    expect(onSelectResource).not.toHaveBeenCalled();
    expect(onOpenObject).not.toHaveBeenCalled();
  });

  it('opens the clicked kind and closes', async () => {
    const user = userEvent.setup();
    const { onSelectResource, onClose } = renderPalette();

    await user.click(await screen.findByRole('button', { name: /Deployment/ }));

    expect(onSelectResource).toHaveBeenCalledWith(expect.objectContaining({ kind: 'Deployment' }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('opens the highlighted entry on Enter', async () => {
    const user = userEvent.setup();
    const { onSelectView } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.click(screen.getByLabelText(/Search resources/));
    await user.keyboard('{Enter}');

    expect(onSelectView).toHaveBeenCalledWith('cluster');
  });

  it('walks the list with the arrow keys', async () => {
    const user = userEvent.setup();
    const { onSelectView } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.click(screen.getByLabelText(/Search resources/));
    await user.keyboard('{ArrowDown}{Enter}');

    expect(onSelectView).toHaveBeenCalledWith('resources');
  });

  it('stops at the ends of the list', async () => {
    const user = userEvent.setup();
    const { onSelectView } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.click(screen.getByLabelText(/Search resources/));
    await user.keyboard('{ArrowUp}{ArrowUp}{Enter}');

    expect(onSelectView).toHaveBeenCalledWith('cluster');
  });

  it('keeps keyboard selection inside the rendered result cap', async () => {
    const user = userEvent.setup();
    const hits = Array.from({ length: 61 }, (_, index) => ({
      ...clusterHit,
      name: `web-${String(index).padStart(2, '0')}`,
    }));
    stubSearch({ hits });
    const { onOpenObject } = renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'web');
    expect(await screen.findByRole('button', { name: /web-59/ })).toBeVisible();
    expect(screen.queryByRole('button', { name: /web-60/ })).not.toBeInTheDocument();
    expect(screen.getByText(/Showing the first 60 matches/)).toBeInTheDocument();
    await user.keyboard('{ArrowDown}'.repeat(80));
    await user.keyboard('{Enter}');

    expect(onOpenObject).toHaveBeenCalledWith({
      ref: {
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'airbyte',
        name: 'web-59',
      },
      type: deploymentType,
      filter: 'web',
      cluster: undefined,
    });
  });

  it('opens a recent object straight from the top of the list', async () => {
    const user = userEvent.setup();
    rememberObject({
      group: '',
      version: 'v1',
      resource: 'pods',
      namespace: 'prod',
      name: 'web-0',
    });
    const { onOpenObject } = renderPalette();

    await user.click(await screen.findByRole('button', { name: /prod\/web-0/ }));

    expect(onOpenObject).toHaveBeenCalledWith({
      ref: { group: '', version: 'v1', resource: 'pods', namespace: 'prod', name: 'web-0' },
      type: podType,
      filter: '',
    });
  });

  it('carries on with just views when discovery fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('discovery down')));
    renderPalette();

    expect(await screen.findByRole('button', { name: /Resources/ })).toBeInTheDocument();
  });

  it('keeps the query between openings, selected so the next word replaces it', async () => {
    const user = userEvent.setup();
    const { view, onClose, onSelectView, onSelectResource, onOpenObject } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });
    await user.type(screen.getByLabelText(/Search resources/), 'deploy');

    view.rerender(
      <CommandPalette
        open={false}
        onClose={onClose}
        onSelectView={onSelectView}
        onSelectResource={onSelectResource}
        onOpenObject={onOpenObject}
      />,
    );

    view.rerender(
      <CommandPalette
        open
        onClose={onClose}
        onSelectView={onSelectView}
        onSelectResource={onSelectResource}
        onOpenObject={onOpenObject}
      />,
    );

    const box = screen.getByLabelText(/Search resources/);
    expect(box).toHaveValue('deploy');
    expect((box as HTMLInputElement).selectionStart).toBe(0);
    expect((box as HTMLInputElement).selectionEnd).toBe('deploy'.length);
  });

  it('forgets the query when the cluster changes', async () => {
    const user = userEvent.setup();
    renderPalette();
    await screen.findByRole('button', { name: /Pod/ });
    await user.type(screen.getByLabelText(/Search resources/), 'deploy');

    act(() => {
      bumpClusterEpoch();
    });

    expect(screen.getByLabelText(/Search resources/)).toHaveValue('');
  });

  it("clears the previous cluster's catalog while the replacement loads", async () => {
    const nextType = makeDescriptor({ resource: 'configmaps', kind: 'ConfigMap' });
    let finishNext!: (response: { ok: boolean; json: () => Promise<unknown> }) => void;
    const next = new Promise<{ ok: boolean; json: () => Promise<unknown> }>((resolve) => {
      finishNext = resolve;
    });
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: () => Promise.resolve({ categories }) })
      .mockImplementationOnce(() => next);
    vi.stubGlobal('fetch', fetchMock);
    renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    act(() => {
      bumpClusterEpoch();
    });

    expect(screen.queryByRole('button', { name: /Pod/ })).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await act(async () => {
      finishNext({
        ok: true,
        json: () => Promise.resolve({ categories: [makeCategory('Config', [nextType])] }),
      });
      await next;
      await Promise.resolve();
    });

    expect(screen.getByRole('button', { name: /ConfigMap/ })).toBeInTheDocument();
  });

  it('does nothing on Enter when nothing matches', async () => {
    const user = userEvent.setup();
    const { onSelectView, onSelectResource, onClose } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.type(screen.getByLabelText(/Search resources/), 'zzzz');
    await user.keyboard('{Enter}');

    expect(onSelectView).not.toHaveBeenCalled();
    expect(onSelectResource).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('ignores a key it has no binding for', async () => {
    const user = userEvent.setup();
    const { onSelectView } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.click(screen.getByLabelText(/Search resources/));
    await user.keyboard('{Tab}');

    expect(onSelectView).not.toHaveBeenCalled();
  });

  it('closes when the click lands outside it', async () => {
    const user = userEvent.setup();
    const { onClose } = renderPalette();

    await user.click(screen.getByRole('dialog', { hidden: true }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('stays open while the click lands inside it', async () => {
    const user = userEvent.setup();
    const { onClose } = renderPalette();

    await user.click(screen.getByLabelText('Search resources, views and recent objects'));

    expect(onClose).not.toHaveBeenCalled();
  });

  it('finds objects in the cluster that are not on screen', async () => {
    const user = userEvent.setup();
    stubSearch({ hits: [clusterHit] });
    renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'airbyte');

    expect(await screen.findByRole('button', { name: /airbyte\/airbyte-server/ })).toBeVisible();
  });

  it('opens the object a cluster hit points at', async () => {
    const user = userEvent.setup();
    stubSearch({ hits: [clusterHit] });
    const { onOpenObject, onClose } = renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'airbyte');
    await user.click(await screen.findByRole('button', { name: /airbyte\/airbyte-server/ }));

    expect(onOpenObject).toHaveBeenCalledWith({
      ref: {
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        namespace: 'airbyte',
        name: 'airbyte-server',
      },
      type: deploymentType,
      filter: 'airbyte',
    });
    expect(onClose).toHaveBeenCalled();
  });

  it('leaves the cluster alone for a single letter', async () => {
    const user = userEvent.setup();
    const asked = stubSearch({ hits: [clusterHit] });
    renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'a');
    await new Promise((resolve) => setTimeout(resolve, 400));

    expect(asked).toHaveLength(0);
  });

  it('says when the sweep could not cover everything', async () => {
    const user = userEvent.setup();
    stubSearch({ hits: [clusterHit], errors: { '/v1/secrets': 'forbidden' } });
    renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'airbyte');

    expect(await screen.findByText(/could not be searched/)).toBeInTheDocument();
  });

  it('keeps the local matches when the sweep fails', async () => {
    const user = userEvent.setup();
    const asked = stubSearch({ ok: false });
    renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'pod');
    await waitFor(() => {
      expect(asked).toHaveLength(1);
    });

    expect(await screen.findByRole('button', { name: /Pod/ })).toBeVisible();
    expect(screen.queryByText(/could not be searched/)).not.toBeInTheDocument();
  });

  it('forgets the cluster hits once the query is cleared', async () => {
    const user = userEvent.setup();
    stubSearch({ hits: [clusterHit] });
    renderPalette();
    await user.type(screen.getByLabelText(/Search resources/), 'airbyte');
    await screen.findByRole('button', { name: /airbyte\/airbyte-server/ });

    await user.clear(screen.getByLabelText(/Search resources/));

    expect(
      screen.queryByRole('button', { name: /airbyte\/airbyte-server/ }),
    ).not.toBeInTheDocument();
  });

  it('ignores a running sweep when the query is cleared', async () => {
    const user = userEvent.setup();
    let release = () => undefined;
    let asked = false;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (!url.startsWith('/api/search')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ categories }),
          });
        }
        asked = true;
        return new Promise((resolve) => {
          release = () => {
            resolve({
              ok: true,
              status: 200,
              json: () => Promise.resolve({ hits: [clusterHit], truncated: false }),
            });
          };
        });
      }),
    );
    renderPalette();
    const input = screen.getByLabelText(/Search resources/);
    await user.type(input, 'airbyte');
    await waitFor(() => {
      expect(asked).toBe(true);
    });

    await user.clear(input);
    release();
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(
      screen.queryByRole('button', { name: /airbyte\/airbyte-server/ }),
    ).not.toBeInTheDocument();
  });

  it('ignores a slow sweep that arrives after a newer one', async () => {
    const user = userEvent.setup();
    const held: { release: () => void } = { release: () => undefined };
    const older = { ...clusterHit, name: 'airbyte-old' };
    let sweeps = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (!url.startsWith('/api/search')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ categories }),
          });
        }
        sweeps += 1;
        if (sweeps === 1) {
          return new Promise((resolve) => {
            held.release = () => {
              resolve({
                ok: true,
                status: 200,
                json: () => Promise.resolve({ hits: [older], truncated: false }),
              });
            };
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ hits: [clusterHit], truncated: false }),
        });
      }),
    );
    renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'air');
    await waitFor(() => {
      expect(sweeps).toBe(1);
    });
    await user.type(screen.getByLabelText(/Search resources/), 'byte');
    await screen.findByRole('button', { name: /airbyte\/airbyte-server/ });
    held.release();

    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(screen.queryByRole('button', { name: /airbyte-old/ })).not.toBeInTheDocument();
  });
});

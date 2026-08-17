import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CommandPalette from '../../src/components/CommandPalette';
import { clearRecents, rememberObject } from '../../src/store/recents';
import { makeCategory, makeDescriptor } from '../helpers';

const categories = [
  makeCategory('Workloads', [
    makeDescriptor({ resource: 'pods', kind: 'Pod' }),
    makeDescriptor({ group: 'apps', resource: 'deployments', kind: 'Deployment' }),
  ]),
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
  const onSelectObject = vi.fn();
  const view = render(
    <CommandPalette
      open={open}
      onClose={onClose}
      onSelectView={onSelectView}
      onSelectResource={onSelectResource}
      onSelectObject={onSelectObject}
    />,
  );
  return { onClose, onSelectView, onSelectResource, onSelectObject, view };
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
  clearRecents();
});

describe('CommandPalette', () => {
  it('lists the discovered kinds once it opens', async () => {
    renderPalette();

    expect(await screen.findByRole('button', { name: /Pod/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Deployment/ })).toBeInTheDocument();
  });

  it('offers every view the cluster can serve', async () => {
    renderPalette();

    expect(await screen.findByRole('button', { name: /Flux graph/ })).toBeInTheDocument();
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
    renderPalette();
    await screen.findByRole('button', { name: /Pod/ });

    await user.type(screen.getByLabelText(/Search resources/), 'zzzz');

    expect(screen.getByText('Nothing matches that.')).toBeInTheDocument();
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

  it('opens a recent object straight from the top of the list', async () => {
    const user = userEvent.setup();
    rememberObject({
      group: '',
      version: 'v1',
      resource: 'pods',
      namespace: 'prod',
      name: 'web-0',
    });
    const { onSelectObject } = renderPalette();

    await user.click(await screen.findByRole('button', { name: /prod\/web-0/ }));

    expect(onSelectObject).toHaveBeenCalledWith(expect.objectContaining({ name: 'web-0' }));
  });

  it('carries on with just views when discovery fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('discovery down')));
    renderPalette();

    expect(await screen.findByRole('button', { name: /Resources/ })).toBeInTheDocument();
  });

  it('forgets the query between openings', async () => {
    const user = userEvent.setup();
    const { view, onClose, onSelectView, onSelectResource, onSelectObject } = renderPalette();
    await screen.findByRole('button', { name: /Pod/ });
    await user.type(screen.getByLabelText(/Search resources/), 'deploy');

    view.rerender(
      <CommandPalette
        open={false}
        onClose={onClose}
        onSelectView={onSelectView}
        onSelectResource={onSelectResource}
        onSelectObject={onSelectObject}
      />,
    );

    expect(screen.getByLabelText(/Search resources/)).toHaveValue('');
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
    const { onSelectObject, onClose } = renderPalette();

    await user.type(screen.getByLabelText(/Search resources/), 'airbyte');
    await user.click(await screen.findByRole('button', { name: /airbyte\/airbyte-server/ }));

    expect(onSelectObject).toHaveBeenCalledWith({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'airbyte',
      name: 'airbyte-server',
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

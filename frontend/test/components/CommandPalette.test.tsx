import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CommandPalette from '../../src/components/CommandPalette';
import { clearRecents, rememberObject } from '../../src/store/recents';
import { makeCategory, makeDescriptor } from '../helpers';

const categories = [
  makeCategory('Workloads', [
    makeDescriptor({ resource: 'pods', kind: 'Pod' }),
    makeDescriptor({ group: 'apps', resource: 'deployments', kind: 'Deployment' }),
  ]),
];

function stubCatalog(): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ categories }) }),
  );
}

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

  it('offers every view', async () => {
    renderPalette();

    expect(await screen.findByRole('button', { name: /GitOps · Graph/ })).toBeInTheDocument();
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
});

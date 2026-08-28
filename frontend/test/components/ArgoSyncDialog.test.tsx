import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ArgoSyncDialog from '../../src/components/ArgoSyncDialog';
import type { ArgoOptions, ArgoResourceRef } from '../../src/lib/argoActions';

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});

const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

function renderDialog(resources?: ArgoResourceRef[]) {
  const onRun = vi.fn<(options: ArgoOptions) => void>();
  const onCancel = vi.fn();
  render(<ArgoSyncDialog name="podinfo" resources={resources} onRun={onRun} onCancel={onCancel} />);
  return { onRun, onCancel };
}

describe('the argo sync options dialog', () => {
  beforeEach(() => {
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
  });

  it('opens as a modal and says what it will sync', () => {
    renderDialog();

    expect(showModal).toHaveBeenCalled();
    expect(
      screen.getByText('Syncing every resource this application manages.'),
    ).toBeInTheDocument();
  });

  it('names the single resource when only one is marked', () => {
    renderDialog([{ kind: 'Deployment', name: 'web' }]);

    expect(screen.getByText('Syncing Deployment web.')).toBeInTheDocument();
  });

  it('counts the marked resources when there are several', () => {
    renderDialog([
      { kind: 'Deployment', name: 'web' },
      { kind: 'Service', name: 'web' },
    ]);

    expect(screen.getByText('Syncing 2 marked resources.')).toBeInTheDocument();
  });

  it('starts with every option off', async () => {
    const user = userEvent.setup();
    const { onRun } = renderDialog();

    await user.click(screen.getByRole('button', { name: 'Synchronize' }));

    expect(onRun).toHaveBeenCalledWith({
      prune: false,
      dryRun: false,
      applyOnly: false,
      force: false,
      replace: false,
      serverSide: false,
      resources: undefined,
    });
  });

  it('carries the marked resources through', async () => {
    const user = userEvent.setup();
    const resources = [{ kind: 'Deployment', name: 'web' }];
    const { onRun } = renderDialog(resources);

    await user.click(screen.getByRole('button', { name: 'Synchronize' }));

    expect(onRun.mock.calls[0][0].resources).toEqual(resources);
  });

  it('turns an option on and off again', async () => {
    const user = userEvent.setup();
    const { onRun } = renderDialog();

    await user.click(screen.getByLabelText(/Prune/));
    await user.click(screen.getByLabelText(/Prune/));
    await user.click(screen.getByRole('button', { name: 'Synchronize' }));

    expect(onRun.mock.calls[0][0].prune).toBe(false);
  });

  it('says the hooks still run when force is on by itself', async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.click(screen.getByLabelText(/Force/));

    expect(screen.getByText('The PreSync and PostSync hooks still run.')).toBeInTheDocument();
  });

  it('drops that line once apply-only is chosen', async () => {
    const user = userEvent.setup();
    renderDialog();

    await user.click(screen.getByLabelText(/Force/));
    await user.click(screen.getByLabelText(/Apply only/));

    expect(screen.queryByText('The PreSync and PostSync hooks still run.')).not.toBeInTheDocument();
  });

  it('cancels without running anything', async () => {
    const user = userEvent.setup();
    const { onRun, onCancel } = renderDialog();

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onCancel).toHaveBeenCalled();
    expect(onRun).not.toHaveBeenCalled();
  });
});

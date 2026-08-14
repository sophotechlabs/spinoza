import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ConfirmByName from '../../src/components/ConfirmByName';

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});
const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

function renderConfirm(open = true) {
  const onConfirm = vi.fn();
  const onCancel = vi.fn();
  const view = render(
    <ConfirmByName
      open={open}
      name="p-mk1"
      what="Deleting Pod web-0."
      onConfirm={onConfirm}
      onCancel={onCancel}
    />,
  );
  return { onConfirm, onCancel, view };
}

beforeEach(() => {
  showModal.mockClear();
  close.mockClear();
  HTMLDialogElement.prototype.showModal = showModal;
  HTMLDialogElement.prototype.close = close;
});

describe('ConfirmByName', () => {
  it('opens as a modal and says what is about to happen', () => {
    renderConfirm();

    expect(showModal).toHaveBeenCalled();
    expect(screen.getByText('Deleting Pod web-0.')).toBeInTheDocument();
    expect(screen.getByText('p-mk1')).toBeInTheDocument();
  });

  it('stays shut until it is asked to open', () => {
    const { view } = renderConfirm(false);

    expect(showModal).not.toHaveBeenCalled();

    view.rerender(
      <ConfirmByName
        open
        name="p-mk1"
        what="Deleting Pod web-0."
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(showModal).toHaveBeenCalledTimes(1);
  });

  it('closes again when it is no longer wanted', () => {
    const { view } = renderConfirm();

    view.rerender(
      <ConfirmByName
        open={false}
        name="p-mk1"
        what="Deleting Pod web-0."
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(close).toHaveBeenCalledTimes(1);
  });

  it('refuses to confirm until the name is typed', async () => {
    const user = userEvent.setup();
    const { onConfirm } = renderConfirm();

    const confirm = screen.getByRole('button', { name: 'Confirm' });
    expect(confirm).toBeDisabled();

    await user.type(screen.getByLabelText('Name'), 'p-mk');
    expect(confirm).toBeDisabled();

    await user.type(screen.getByLabelText('Name'), '1');
    expect(confirm).toBeEnabled();

    await user.click(confirm);
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('gives up on cancel', async () => {
    const user = userEvent.setup();
    const { onCancel } = renderConfirm();

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('treats a dialog dismissed with escape as a cancel', () => {
    const { onCancel } = renderConfirm();

    screen.getByRole('dialog', { hidden: true }).dispatchEvent(new Event('close'));

    expect(onCancel).toHaveBeenCalledTimes(1);
  });

  it('forgets what was typed the next time it opens', async () => {
    const user = userEvent.setup();
    const { view } = renderConfirm();

    await user.type(screen.getByLabelText('Name'), 'p-mk1');
    view.rerender(
      <ConfirmByName
        open={false}
        name="p-mk1"
        what="Deleting Pod web-0."
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );
    view.rerender(
      <ConfirmByName
        open
        name="p-mk1"
        what="Deleting Pod web-0."
        onConfirm={vi.fn()}
        onCancel={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('Name')).toHaveValue('');
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  });
});

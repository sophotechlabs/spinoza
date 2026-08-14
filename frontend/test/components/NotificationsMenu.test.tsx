import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import NotificationsMenu from '../../src/components/NotificationsMenu';
import type { ObjectRef } from '../../src/lib/types';
import { notifyOk, useToastsStore } from '../../src/store/toasts';

const podRef: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web-0',
};

function menu() {
  const onSelectObject = vi.fn();
  const view = render(<NotificationsMenu onSelectObject={onSelectObject} />);
  return { onSelectObject, view };
}

function bell(): HTMLElement {
  return screen.getByLabelText('Notifications');
}

function unfolded(): boolean {
  const details = bell().closest('details');
  return details?.open === true;
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

describe('NotificationsMenu', () => {
  it('sits folded away behind a bell', () => {
    notifyOk('Deleted Pod web-0');
    menu();

    expect(unfolded()).toBe(false);
    expect(bell()).toHaveAccessibleName('Notifications');
  });

  it('unfolds the history when the bell is clicked', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0');
    menu();

    await user.click(bell());

    expect(unfolded()).toBe(true);
    expect(screen.getByText('Deleted Pod web-0')).toBeInTheDocument();
  });

  it('folds away again on a second click', async () => {
    const user = userEvent.setup();
    menu();

    await user.click(bell());
    await user.click(bell());

    expect(unfolded()).toBe(false);
  });

  it('opens the object an entry came from and folds itself away', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0', podRef);
    const { onSelectObject } = menu();

    await user.click(bell());
    await user.click(screen.getByRole('button', { name: 'pods/prod/web-0' }));

    expect(onSelectObject).toHaveBeenCalledWith(podRef);
    expect(unfolded()).toBe(false);
  });
});

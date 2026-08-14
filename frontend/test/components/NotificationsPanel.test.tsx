import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import NotificationsPanel from '../../src/components/NotificationsPanel';
import type { ObjectRef } from '../../src/lib/types';
import { notifyError, notifyOk, notifyWarn, useToastsStore } from '../../src/store/toasts';

const podRef: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web-0',
};

const nodeRef: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'nodes',
  namespace: '',
  name: 'worker-1',
};

function panel() {
  const onSelectObject = vi.fn();
  const view = render(<NotificationsPanel onSelectObject={onSelectObject} />);
  return { onSelectObject, view };
}

function rows(): HTMLElement[] {
  return screen.getAllByRole('listitem');
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

describe('NotificationsPanel', () => {
  it('says so while nothing has happened', () => {
    panel();

    expect(screen.getByText('Nothing to show here yet.')).toBeInTheDocument();
    expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  });

  it('lists what happened, newest first', () => {
    notifyOk('Deleted Pod web-0');
    notifyError('drain worker-1: 2 pods cannot be evicted');
    panel();

    expect(rows()[0]).toHaveTextContent('drain worker-1');
    expect(rows()[1]).toHaveTextContent('Deleted Pod web-0');
  });

  it('stamps each entry with the time it arrived', () => {
    notifyOk('Deleted Pod web-0');
    panel();

    expect(rows()[0].textContent).toMatch(/\d\d:\d\d:\d\d/);
  });

  it('marks the tone of each entry', () => {
    notifyWarn('The kubeconfig stopped reading');
    panel();

    expect(within(rows()[0]).getByRole('img', { name: 'warn' })).toBeInTheDocument();
  });

  it('counts everything since the cluster was opened', () => {
    notifyOk('one');
    notifyOk('two');
    panel();

    expect(screen.getByText('2 since this cluster was opened, newest first')).toBeInTheDocument();
  });

  it('narrows to one tone when asked', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0');
    notifyError('drain worker-1 failed');
    panel();

    await user.click(screen.getByRole('button', { name: 'Failures' }));

    expect(rows()).toHaveLength(1);
    expect(rows()[0]).toHaveTextContent('drain worker-1 failed');
    expect(screen.getByRole('button', { name: 'Failures' })).toHaveAttribute(
      'aria-pressed',
      'true',
    );
  });

  it('says when a tone has nothing in it', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0');
    panel();

    await user.click(screen.getByRole('button', { name: 'Warnings' }));

    expect(screen.getByText('Nothing to show here yet.')).toBeInTheDocument();
  });

  it('goes back to everything', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0');
    notifyError('drain worker-1 failed');
    panel();

    await user.click(screen.getByRole('button', { name: 'Failures' }));
    await user.click(screen.getByRole('button', { name: 'All' }));

    expect(rows()).toHaveLength(2);
  });

  it('opens the object an entry came from', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0', podRef);
    const { onSelectObject } = panel();

    await user.click(screen.getByRole('button', { name: 'pods/prod/web-0' }));

    expect(onSelectObject).toHaveBeenCalledWith(podRef);
  });

  it('leaves the namespace out for a cluster-wide object', () => {
    notifyOk('Cordoned worker-1', nodeRef);
    panel();

    expect(screen.getByRole('button', { name: 'nodes/worker-1' })).toBeInTheDocument();
  });

  it('offers nothing to open for an entry with no object', () => {
    notifyOk('Switched to p-mk2');
    panel();

    expect(within(rows()[0]).queryByRole('button')).not.toBeInTheDocument();
  });

  it('empties the list on request', async () => {
    const user = userEvent.setup();
    notifyOk('Deleted Pod web-0');
    panel();

    await user.click(screen.getByRole('button', { name: 'Clear' }));

    expect(screen.getByText('Nothing to show here yet.')).toBeInTheDocument();
    expect(useToastsStore.getState().history).toHaveLength(0);
  });

  it('has nothing to clear while the list is empty', () => {
    panel();

    expect(screen.getByRole('button', { name: 'Clear' })).toBeDisabled();
  });
});

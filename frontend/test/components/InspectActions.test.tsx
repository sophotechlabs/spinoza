import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectActions from '../../src/components/InspectActions';
import type { ObjectRef } from '../../src/lib/types';

const target: ObjectRef = {
  group: 'kustomize.toolkit.fluxcd.io',
  version: 'v1',
  resource: 'kustomizations',
  namespace: 'flux-system',
  name: 'apps',
};

function renderActions(suspended?: boolean) {
  const onDone = vi.fn();
  const view = render(<InspectActions target={target} suspended={suspended} onDone={onDone} />);
  return { onDone, view };
}

function lastCallUrl(): string {
  const mock = globalThis.fetch as ReturnType<typeof vi.fn>;
  return mock.mock.calls[mock.mock.calls.length - 1][0] as string;
}

describe('InspectActions', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({}) }),
    );
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('offers reconcile and suspend for a running resource', () => {
    renderActions(false);

    expect(screen.getByRole('button', { name: 'Reconcile' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Suspend' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
    expect(screen.queryByText('suspended')).not.toBeInTheDocument();
  });

  it('offers resume for a suspended resource', () => {
    renderActions(true);

    expect(screen.getByRole('button', { name: 'Resume' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Suspend' })).not.toBeInTheDocument();
    expect(screen.getByText('suspended')).toBeInTheDocument();
  });

  it('treats an unknown suspend state as running', () => {
    renderActions(undefined);

    expect(screen.getByRole('button', { name: 'Suspend' })).toBeInTheDocument();
  });

  it('requests a reconcile and reports it', async () => {
    const user = userEvent.setup();
    const { onDone } = renderActions(false);

    await user.click(screen.getByRole('button', { name: 'Reconcile' }));

    expect(await screen.findByText('Reconciliation requested.')).toBeInTheDocument();
    expect(lastCallUrl()).toContain('action=reconcile');
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('suspends and reports it', async () => {
    const user = userEvent.setup();
    const { onDone } = renderActions(false);

    await user.click(screen.getByRole('button', { name: 'Suspend' }));

    expect(await screen.findByText('Suspended.')).toBeInTheDocument();
    expect(lastCallUrl()).toContain('action=suspend');
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('resumes and reports it', async () => {
    const user = userEvent.setup();
    const { onDone } = renderActions(true);

    await user.click(screen.getByRole('button', { name: 'Resume' }));

    expect(await screen.findByText('Resumed.')).toBeInTheDocument();
    expect(lastCallUrl()).toContain('action=resume');
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('surfaces a failure and does not report success', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
        json: () => Promise.resolve({ message: 'forbidden' }),
      }),
    );
    const { onDone } = renderActions(false);

    await user.click(screen.getByRole('button', { name: 'Reconcile' }));

    expect(await screen.findByText('forbidden')).toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderActions(false);

    await user.click(screen.getByRole('button', { name: 'Reconcile' }));

    expect(await screen.findByText('action failed')).toBeInTheDocument();
  });

  it('clears feedback when the target changes', async () => {
    const user = userEvent.setup();
    const { view } = renderActions(false);
    await user.click(screen.getByRole('button', { name: 'Reconcile' }));
    await screen.findByText('Reconciliation requested.');

    view.rerender(
      <InspectActions target={{ ...target, name: 'infra' }} suspended={false} onDone={vi.fn()} />,
    );

    expect(screen.queryByText('Reconciliation requested.')).not.toBeInTheDocument();
  });

  it('disables the buttons while an action is in flight', async () => {
    const user = userEvent.setup();
    const deferred = {
      release: () => {
        return undefined;
      },
    };
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(
        () =>
          new Promise((resolve) => {
            deferred.release = () => {
              resolve({ ok: true, json: () => Promise.resolve({}) });
            };
          }),
      ),
    );
    renderActions(false);

    await user.click(screen.getByRole('button', { name: 'Reconcile' }));

    expect(await screen.findByText('working…')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reconcile' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Suspend' })).toBeDisabled();

    deferred.release();
    expect(await screen.findByText('Reconciliation requested.')).toBeInTheDocument();
  });
});

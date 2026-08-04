import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectObjectActions from '../../src/components/InspectObjectActions';
import { useToastsStore } from '../../src/store/toasts';
import type { ObjectDetail, ObjectRef } from '../../src/lib/types';

const deployment: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'shop',
  name: 'web',
};

const daemonSet: ObjectRef = { ...deployment, resource: 'daemonsets' };

const node: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'nodes',
  namespace: '',
  name: 'worker-1',
};

function detailFor(overrides: Partial<ObjectDetail> = {}): ObjectDetail {
  return {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    name: 'web',
    namespace: 'shop',
    uid: 'uid',
    createdAt: '2026-07-31T10:00:00Z',
    yaml: '',
    ...overrides,
  };
}

function stub(...bodies: unknown[]) {
  const fetchMock = vi.fn();
  for (const body of bodies) {
    fetchMock.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(body) });
  }
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function stubFailure(message: string) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: false,
    status: 400,
    json: () => Promise.resolve({ message }),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('scale', () => {
  it('seeds the input with the current replica count', () => {
    stub();
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('replicas')).toHaveValue(3);
  });

  it('scales to the typed count and reports the result', async () => {
    const user = userEvent.setup();
    const onDone = vi.fn();
    const fetchMock = stub({ action: 'scale', message: 'Scaled web to 5 replicas.' });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={onDone}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '5');
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(await screen.findByText('Scaled web to 5 replicas.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('replicas=5');
    expect(onDone).toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'web: Scaled web to 5 replicas.' }),
    ]);
  });

  it('refuses a fractional count without calling the server', async () => {
    const user = userEvent.setup();
    const fetchMock = stub();
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '1.5');
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(await screen.findByText(/whole number/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('refuses an empty count', async () => {
    const user = userEvent.setup();
    const fetchMock = stub();
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 2 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(await screen.findByText(/whole number/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('surfaces a server refusal', async () => {
    const user = userEvent.setup();
    stubFailure('deployments.apps "web" is forbidden');
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(await screen.findByText('deployments.apps "web" is forbidden')).toBeInTheDocument();
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'error',
        message: 'scale web: deployments.apps "web" is forbidden',
      }),
    ]);
  });
});

describe('restart', () => {
  it('requests a rollout restart', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'restart', message: 'Rollout restart requested at X.' });
    render(<InspectObjectActions target={daemonSet} detail={detailFor()} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Restart' }));

    expect(await screen.findByText('Rollout restart requested at X.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('action=restart');
  });

  it('offers no scale control for a daemonset', () => {
    stub();
    render(<InspectObjectActions target={daemonSet} detail={detailFor()} onDone={vi.fn()} />);

    expect(screen.queryByLabelText('replicas')).not.toBeInTheDocument();
  });
});

describe('node actions', () => {
  it('cordons a schedulable node', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'cordon', message: 'worker-1 no longer accepts new pods.' });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Cordon' }));

    expect(await screen.findByText('worker-1 no longer accepts new pods.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('action=cordon');
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({
        tone: 'ok',
        message: 'worker-1: worker-1 no longer accepts new pods.',
      }),
    ]);
  });

  it('offers uncordon on a cordoned node', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'uncordon', message: 'worker-1 accepts new pods.' });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: false } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByText('cordoned')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Cordon' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Uncordon' }));

    expect(String(fetchMock.mock.calls[0][0])).toContain('action=uncordon');
  });

  it('shows the plan before draining anything', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({
      action: 'drain',
      dryRun: true,
      message: '1 pod to evict, 1 left in place.',
      pods: [
        { namespace: 'shop', name: 'web-1', outcome: 'evict' },
        { namespace: 'kube-system', name: 'logger', outcome: 'skipped', reason: 'daemonset pod' },
      ],
    });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));

    expect(await screen.findByText('1 pod to evict, 1 left in place.')).toBeInTheDocument();
    expect(screen.getByText('web-1')).toBeInTheDocument();
    expect(screen.getByText('daemonset pod')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('dryRun=true');
  });

  it('drains after the plan is confirmed', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(
      {
        action: 'drain',
        dryRun: true,
        message: '1 pod to evict.',
        pods: [{ namespace: 'shop', name: 'web-1', outcome: 'evict' }],
      },
      { action: 'drain', message: 'Cordoned. Eviction requested for 1 pod.' },
    );
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));
    await user.click(await screen.findByRole('button', { name: 'Drain now' }));

    expect(await screen.findByText('Cordoned. Eviction requested for 1 pod.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[1][0])).not.toContain('dryRun');
    expect(screen.queryByRole('button', { name: 'Drain now' })).not.toBeInTheDocument();
  });

  it('holds the drain until the blocked pods are acknowledged', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({
      action: 'drain',
      dryRun: true,
      message: '0 pods to evict, 1 blocked.',
      pods: [
        {
          namespace: 'shop',
          name: 'scratch',
          outcome: 'blocked',
          reason: 'no controller owns it, nothing would recreate it',
        },
      ],
    });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));
    const confirm = await screen.findByRole('button', { name: 'Drain now' });
    expect(confirm).toBeDisabled();

    await user.click(screen.getByRole('checkbox'));

    expect(confirm).toBeEnabled();
    await user.click(confirm);
    await waitFor(() => {
      expect(String(fetchMock.mock.calls[1][0])).toContain('force=true');
    });
  });

  it('drops the plan on cancel', async () => {
    const user = userEvent.setup();
    stub({
      action: 'drain',
      dryRun: true,
      message: '1 pod to evict.',
      pods: [{ namespace: 'shop', name: 'web-1', outcome: 'evict' }],
    });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));
    await user.click(await screen.findByRole('button', { name: 'Cancel' }));

    expect(screen.queryByRole('button', { name: 'Drain now' })).not.toBeInTheDocument();
  });

  it('reports a drain the server refused', async () => {
    const user = userEvent.setup();
    stubFailure('2 pods cannot be evicted safely');
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));

    expect(await screen.findByText('2 pods cannot be evicted safely')).toBeInTheDocument();
  });

  it('colours each outcome and shows a failed one', async () => {
    const user = userEvent.setup();
    stub({
      action: 'drain',
      dryRun: true,
      message: 'mixed',
      pods: [
        { namespace: 'a', name: 'one', outcome: 'evict' },
        { namespace: 'a', name: 'two', outcome: 'blocked' },
        { namespace: 'a', name: 'three', outcome: 'skipped' },
        { namespace: 'a', name: 'four', outcome: 'failed' },
      ],
    });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));

    expect(await screen.findByText('failed')).toBeInTheDocument();
    expect(screen.getByText('blocked')).toBeInTheDocument();
    expect(screen.getByText('skipped')).toBeInTheDocument();
    expect(screen.getByText('evict')).toBeInTheDocument();
  });

  it('offers no scale or restart on a node', () => {
    stub();
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.queryByLabelText('replicas')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();
  });
});

describe('a rejection that is not an Error', () => {
  it('falls back to a generic message', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(await screen.findByText('action failed')).toBeInTheDocument();
  });
});

describe('switching objects', () => {
  it('reseeds the replica count and clears the last message', async () => {
    const user = userEvent.setup();
    stub({ action: 'scale', message: 'Scaled web to 1 replica.' });
    const view = render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Scale' }));
    expect(await screen.findByText('Scaled web to 1 replica.')).toBeInTheDocument();

    view.rerender(
      <InspectObjectActions
        target={{ ...deployment, name: 'api' }}
        detail={detailFor({ name: 'api', workload: { replicas: 7 } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('replicas')).toHaveValue(7);
    expect(screen.queryByText('Scaled web to 1 replica.')).not.toBeInTheDocument();
  });

  it('keeps the message when the same object is refetched', async () => {
    const user = userEvent.setup();
    stub({ action: 'scale', message: 'Scaled web to 5 replicas.' });
    const view = render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '5');
    await user.click(screen.getByRole('button', { name: 'Scale' }));
    expect(await screen.findByText('Scaled web to 5 replicas.')).toBeInTheDocument();

    view.rerender(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 5 } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByText('Scaled web to 5 replicas.')).toBeInTheDocument();
    expect(screen.getByLabelText('replicas')).toHaveValue(5);
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import InspectObjectActions from '../../src/components/InspectObjectActions';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';
import { accessKey, useAccessStore } from '../../src/store/access';
import { contextScope, EMPTY_CONTEXTS } from '../../src/store/contexts';
import type { ObjectDetail, ObjectRef } from '../../src/lib/types';
import { bumpClusterEpoch } from '../../src/store/cluster';

const deployment: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'shop',
  name: 'web',
};

const daemonSet: ObjectRef = { ...deployment, resource: 'daemonsets' };

const cronJob: ObjectRef = {
  group: 'batch',
  version: 'v1',
  resource: 'cronjobs',
  namespace: 'shop',
  name: 'nightly',
};

const node: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'nodes',
  namespace: '',
  name: 'worker-1',
};

const clusterScope = contextScope({ kubeconfig: '', name: 'p-mk1' }, 0);

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

const NODE_SHELL_SUPPORT = {
  node: 'worker-1',
  enabled: false,
  allowed: false,
  reason: 'node shells are off',
  image: 'busybox:1.37',
  namespace: 'kube-system',
};

function answersSupport(url: unknown): boolean {
  return typeof url === 'string' && url.startsWith('/api/nodeshell/support');
}

function stub(...bodies: unknown[]) {
  const actions = vi.fn<(url: unknown, init?: unknown) => Promise<unknown>>();
  for (const body of bodies) {
    actions.mockResolvedValueOnce({ ok: true, json: () => Promise.resolve(body) });
  }
  const fetchMock = vi.fn((url: unknown, init?: unknown): Promise<unknown> => {
    if (answersSupport(url)) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(NODE_SHELL_SUPPORT) });
    }
    return actions(url, init);
  });
  vi.stubGlobal('fetch', fetchMock);
  return actions;
}

function stubFailure(message: string) {
  const fetchMock = vi.fn((url: unknown) => {
    if (answersSupport(url)) {
      return Promise.resolve({ ok: true, json: () => Promise.resolve(NODE_SHELL_SUPPORT) });
    }
    return Promise.resolve({ ok: false, status: 400, json: () => Promise.resolve({ message }) });
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
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('Rollout restart requested at X.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('action=restart');
  });

  it('offers no scale control for a daemonset', () => {
    stub();
    render(<InspectObjectActions target={daemonSet} detail={detailFor()} onDone={vi.fn()} />);

    expect(screen.queryByLabelText('replicas')).not.toBeInTheDocument();
  });
});

describe('cron job actions', () => {
  it('suspends a running cron job once confirmed', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'suspend', message: 'Suspended nightly.' });
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: false })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Suspend' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('Suspended nightly.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('action=suspend');
  });

  it('asks before suspending', async () => {
    const user = userEvent.setup();
    const fetchMock = stub();
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: false })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Suspend' }));

    expect(screen.getByText(/No new runs are started/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('resumes a suspended cron job without asking', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'resume', message: 'Resumed nightly.' });
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: true })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Resume' }));

    expect(await screen.findByText('Resumed nightly.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('action=resume');
  });

  it('offers only one of the two, and says which state it is in', () => {
    stub();
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: true })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.queryByRole('button', { name: 'Suspend' })).not.toBeInTheDocument();
    expect(screen.getByText('suspended')).toBeInTheDocument();
  });

  it('starts a run outside the schedule once confirmed', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'trigger', message: 'Started job nightly-1756400000.' });
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: false })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Run now' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('Started job nightly-1756400000.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('action=trigger');
  });

  it('asks before starting a run', async () => {
    const user = userEvent.setup();
    const fetchMock = stub();
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: false })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Run now' }));

    expect(screen.getByText(/outside the schedule/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('offers a run even while the schedule is suspended', () => {
    stub();
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: true })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Run now' })).toBeEnabled();
  });

  it('leaves a workload alone', () => {
    stub();
    render(<InspectObjectActions target={daemonSet} detail={detailFor()} onDone={vi.fn()} />);

    expect(screen.queryByRole('button', { name: 'Suspend' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Resume' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Run now' })).not.toBeInTheDocument();
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
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

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

describe('the actions that cannot be undone', () => {
  it('asks before restarting and does nothing if you back out', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'restart', message: 'done' });
    render(<InspectObjectActions target={daemonSet} detail={detailFor()} onDone={vi.fn()} />);

    await user.click(screen.getByRole('button', { name: 'Restart' }));
    expect(screen.getByText(/Restart web\? Every pod is replaced\./)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByText(/Every pod is replaced/)).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('asks before cordoning', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'cordon', message: 'done' });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Cordon' }));

    expect(screen.getByText(/Nothing new will be scheduled on it/)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('asks before scaling to zero', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'scale', message: 'scaled' });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '0');
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(screen.getByText(/Scale web to zero\? Every pod is removed\./)).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('scaled')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('replicas=0');
  });

  it('scales to a positive count without asking', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'scale', message: 'scaled' });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '4');
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(await screen.findByText('scaled')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[0][0])).toContain('replicas=4');
  });

  it('uncordons without asking, since it only widens what is allowed', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'uncordon', message: 'back in service' });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: false } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Uncordon' }));

    expect(await screen.findByText('back in service')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('forgets a pending question when the target changes', async () => {
    const user = userEvent.setup();
    stub({ action: 'restart', message: 'done' });
    const view = render(
      <InspectObjectActions target={daemonSet} detail={detailFor()} onDone={vi.fn()} />,
    );
    await user.click(screen.getByRole('button', { name: 'Restart' }));

    view.rerender(
      <InspectObjectActions
        target={{ ...daemonSet, name: 'other' }}
        detail={detailFor()}
        onDone={vi.fn()}
      />,
    );

    expect(screen.queryByText(/Every pod is replaced/)).not.toBeInTheDocument();
  });

  it('drops an action result when the target changes', async () => {
    const user = userEvent.setup();
    let finish: (body: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      ),
    );
    const onDone = vi.fn();
    const view = render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={onDone}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Scale' }));
    await screen.findByText('working');

    view.rerender(
      <InspectObjectActions
        target={{ ...deployment, name: 'other' }}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );
    expect(screen.queryByText('working')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Scale' })).toBeEnabled();

    finish({ ok: true, status: 200, json: () => Promise.resolve({ message: 'scaled' }) });
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(screen.queryByText('scaled')).not.toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('drops an action failure when the target changes', async () => {
    const user = userEvent.setup();
    let fail: (reason?: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            fail = reject;
          }),
      ),
    );
    const onDone = vi.fn();
    const view = render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={onDone}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Scale' }));
    await screen.findByText('working');

    view.rerender(
      <InspectObjectActions
        target={{ ...deployment, name: 'other' }}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={vi.fn()}
      />,
    );
    await act(async () => {
      fail(new Error('the old action failed'));
      await Promise.resolve();
    });

    expect(screen.queryByText('the old action failed')).not.toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });

  it('drops an action result after the cluster reconnects', async () => {
    const user = userEvent.setup();
    let finish!: (response: { ok: boolean; status: number; json: () => Promise<unknown> }) => void;
    const action = new Promise<{
      ok: boolean;
      status: number;
      json: () => Promise<unknown>;
    }>((resolve) => {
      finish = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/action')) {
          return action;
        }
        return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({}) });
      }),
    );
    const onDone = vi.fn();
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 1 } })}
        onDone={onDone}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Scale' }));
    await screen.findByText('working');

    act(() => {
      bumpClusterEpoch();
    });
    expect(screen.queryByText('working')).not.toBeInTheDocument();

    await act(async () => {
      finish({ ok: true, status: 200, json: () => Promise.resolve({ message: 'scaled' }) });
      await action;
      await Promise.resolve();
    });

    expect(screen.queryByText('scaled')).not.toBeInTheDocument();
    expect(onDone).not.toHaveBeenCalled();
    expect(useToastsStore.getState().toasts).toHaveLength(0);
  });
});

describe('on a protected cluster', () => {
  const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
    this.open = true;
  });
  const close = vi.fn(function close(this: HTMLDialogElement) {
    this.open = false;
  });

  beforeEach(() => {
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'p-mk1' },
      kubeconfigs: [],
      protection: 'protected',
    });
  });

  it('asks for the name before scaling to zero', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'scale', message: 'Scaled web to 0 replicas.' });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '0');
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    expect(screen.getByText('Scale web to zero? Every pod is removed.')).toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();

    await user.type(screen.getByLabelText('Name'), 'web');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });
    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('replicas=0');
    expect(url).toContain('confirm=web');
  });

  it('scales up without a word', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'scale', message: 'Scaled web to 5 replicas.' });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    await user.clear(screen.getByLabelText('replicas'));
    await user.type(screen.getByLabelText('replicas'), '5');
    await user.click(screen.getByRole('button', { name: 'Scale' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });
    expect(String(fetchMock.mock.calls[0][0])).not.toContain('confirm');
  });

  it('asks for the name between the drain plan and the drain', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(
      { action: 'drain', message: '1 pod would be evicted.', dryRun: true, pods: [] },
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

    expect(screen.getByText('Draining node worker-1 evicts its pods.')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await user.type(screen.getByLabelText('Name'), 'worker-1');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('Cordoned. Eviction requested for 1 pod.')).toBeInTheDocument();
    expect(String(fetchMock.mock.calls[1][0])).toContain('confirm=worker-1');
  });

  it('leaves the drain alone when the question is cancelled', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({
      action: 'drain',
      message: '1 pod would be evicted.',
      dryRun: true,
      pods: [],
    });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Drain' }));
    await user.click(await screen.findByRole('button', { name: 'Drain now' }));
    const dialog = screen.getByRole('dialog', { hidden: true });
    await user.click(within(dialog).getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('still cordons and restarts with one click', async () => {
    const user = userEvent.setup();
    const fetchMock = stub({ action: 'cordon', message: 'Cordoned worker-1.' });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'Cordon' }));
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });
    expect(String(fetchMock.mock.calls[0][0])).not.toContain('confirm');
  });
});

describe('an action the cluster would refuse', () => {
  const deploymentKey = accessKey(clusterScope, deployment);
  const nodeKey = accessKey(clusterScope, node);

  beforeEach(() => {
    useAccessStore.getState().forget();
    useContextsStore.getState().setList({
      ...EMPTY_CONTEXTS,
      current: { kubeconfig: '', name: 'p-mk1' },
    });
  });

  afterEach(() => {
    useAccessStore.getState().forget();
  });

  it('greys out Scale and says why', () => {
    stub();
    useAccessStore.getState().setRefused(deploymentKey, {
      scale: 'requires container.deployments.update in Cloud IAM',
    });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    const button = screen.getByRole('button', { name: 'Scale' });
    expect(button).toBeDisabled();
    expect(button).toHaveAccessibleDescription(
      'Scale unavailable: requires container.deployments.update in Cloud IAM',
    );
    expect(
      screen.getByText(/Scale unavailable: requires container.deployments.update in Cloud IAM/),
    ).toBeVisible();
  });

  it('greys out Restart and says why', () => {
    stub();
    useAccessStore.getState().setRefused(deploymentKey, { restart: 'no patching here' });
    render(<InspectObjectActions target={deployment} detail={detailFor()} onDone={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Restart' })).toBeDisabled();
  });

  it('greys out Cordon and Drain on a node', () => {
    stub();
    useAccessStore.getState().setRefused(nodeKey, {
      cordon: 'no patching nodes',
      drain: 'no evicting pods',
    });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: true } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Cordon' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Drain' })).toHaveAccessibleDescription(
      'Drain unavailable: no evicting pods',
    );
  });

  it('greys out Uncordon on a node that is already cordoned', () => {
    stub();
    useAccessStore.getState().setRefused(nodeKey, { cordon: 'no patching nodes' });
    render(
      <InspectObjectActions
        target={node}
        detail={detailFor({ kind: 'Node', node: { schedulable: false } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Uncordon' })).toBeDisabled();
  });

  it('greys out Suspend and says why', () => {
    stub();
    useAccessStore
      .getState()
      .setRefused(accessKey(clusterScope, cronJob), { suspend: 'no patching cron jobs' });
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: false })}
        onDone={vi.fn()}
      />,
    );

    const button = screen.getByRole('button', { name: 'Suspend' });
    expect(button).toBeDisabled();
    expect(button).toHaveAccessibleDescription('Suspend unavailable: no patching cron jobs');
  });

  it('greys out Run now on its own refusal', () => {
    stub();
    useAccessStore
      .getState()
      .setRefused(accessKey(clusterScope, cronJob), { trigger: 'no creating jobs in prod' });
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: false })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Run now' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Suspend' })).toBeEnabled();
  });

  it('greys out Resume with the same refusal', () => {
    stub();
    useAccessStore
      .getState()
      .setRefused(accessKey(clusterScope, cronJob), { suspend: 'no patching cron jobs' });
    render(
      <InspectObjectActions
        target={cronJob}
        detail={detailFor({ kind: 'CronJob', name: 'nightly', suspended: true })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Resume' })).toBeDisabled();
  });

  it('leaves the buttons alone when the refusals are about another object', () => {
    stub();
    useAccessStore.getState().setRefused(nodeKey, { scale: 'not about this deployment' });
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Scale' })).toBeEnabled();
  });

  it('leaves the buttons alone when nothing is refused', () => {
    stub();
    useAccessStore.getState().setRefused(deploymentKey, {});
    render(
      <InspectObjectActions
        target={deployment}
        detail={detailFor({ workload: { replicas: 3 } })}
        onDone={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Scale' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Scale' })).not.toHaveAttribute('aria-describedby');
  });
});

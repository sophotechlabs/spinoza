import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import RolloutRevisions from '../../src/components/RolloutRevisions';
import type { ObjectRef, RevisionDiff, Revisions } from '../../src/lib/types';
import { useContextsStore } from '../../src/store/contexts';
import { useToastsStore } from '../../src/store/toasts';

function noop() {
  return undefined;
}

vi.mock('../../src/components/YamlDiff', () => ({
  default: ({ left, right }: { left: string; right: string }) => (
    <div data-testid="diff">{`${left}|${right}`}</div>
  ),
}));

const target: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'prod',
  name: 'web',
};

const answer: Revisions = {
  supported: true,
  revisions: [
    {
      number: 3,
      name: 'web-abc',
      current: true,
      createdAt: '2026-09-09T10:00:00Z',
      images: ['nginx:1.27'],
    },
    {
      number: 2,
      name: 'web-def',
      createdAt: '2026-09-08T10:00:00Z',
      images: ['nginx:1.26'],
      cause: 'kubectl set image',
    },
  ],
};

const difference: RevisionDiff = {
  from: 2,
  to: 3,
  lines: 2,
  left: 'image: nginx:1.26',
  right: 'image: nginx:1.27',
};

function route(answers: Record<string, unknown>, failures: Record<string, number> = {}) {
  const calls: string[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn((url: string) => {
      calls.push(url);
      const key = Object.keys(answers).find((one) => url.includes(one));
      const failed = Object.keys(failures).find((one) => url.includes(one));
      if (failed !== undefined) {
        return Promise.resolve({
          ok: false,
          status: failures[failed],
          json: () => Promise.resolve({ message: 'the cluster said no' }),
          text: () => Promise.resolve('{}'),
        });
      }
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(key === undefined ? {} : answers[key]),
        text: () => Promise.resolve('{}'),
      });
    }),
  );
  return calls;
}

function stubDialog() {
  HTMLDialogElement.prototype.showModal = function showModal(this: HTMLDialogElement) {
    this.open = true;
  };
  HTMLDialogElement.prototype.close = function close(this: HTMLDialogElement) {
    this.open = false;
  };
}

function protectedCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'protected',
  });
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  useContextsStore.getState().reset();
  useToastsStore.getState().clear();
});

describe('RolloutRevisions', () => {
  it('lists what rolled out and marks the one running now', async () => {
    route({ '/api/rollout': answer });

    render(<RolloutRevisions target={target} onDone={noop} />);

    expect(await screen.findByText('#3')).toBeInTheDocument();
    expect(screen.getByText('current')).toBeInTheDocument();
    expect(screen.getByText('#2')).toBeInTheDocument();
    expect(screen.getByText('nginx:1.26')).toBeInTheDocument();
    expect(screen.getByText('kubectl set image')).toBeInTheDocument();
  });

  it('says so plainly when the kind keeps no revisions', async () => {
    route({
      '/api/rollout': {
        supported: false,
        revisions: [],
        reason: 'configmaps keep no rollout revisions',
      },
    });

    render(<RolloutRevisions target={target} onDone={noop} />);

    expect(await screen.findByText(/configmaps keep no rollout revisions/)).toBeInTheDocument();
  });

  it('shows the diff between a revision and the one running now', async () => {
    route({ '/api/rollout/diff': difference, '/api/rollout': answer });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Diff' }));

    await waitFor(() => {
      expect(screen.getByTestId('diff')).toHaveTextContent('image: nginx:1.26|image: nginx:1.27');
    });
    expect(screen.getByText('#2 against #3: 2 lines differ')).toBeInTheDocument();
  });

  it('says the templates are the same rather than drawing an empty diff', async () => {
    route({
      '/api/rollout/diff': { from: 2, to: 3, lines: 0, same: true, left: 'a', right: 'a' },
      '/api/rollout': answer,
    });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Diff' }));

    expect(await screen.findByText('#2 and #3 hold the same pod template')).toBeInTheDocument();
    expect(screen.queryByTestId('diff')).not.toBeInTheDocument();
  });

  it('sends the revision it was asked to go back to', async () => {
    const calls = route({ '/api/rollout': answer, '/api/action': { message: 'went back to 2' } });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Go back to this' }));

    await waitFor(() => {
      const undo = calls.find((one) => one.includes('/api/action'));
      expect(undo).toBeDefined();
      expect(undo).toContain('action=undo');
      expect(undo).toContain('revision=2');
    });
  });

  it('offers no way back from the revision already running', async () => {
    route({ '/api/rollout': answer });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#3');

    expect(screen.getAllByRole('button', { name: 'Go back to this' })).toHaveLength(1);
    expect(screen.getAllByRole('button', { name: 'Diff' })).toHaveLength(1);
  });

  it('names what the cluster refused rather than an empty diff', async () => {
    route({ '/api/rollout': answer }, { '/api/rollout/diff': 403 });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Diff' }));

    expect(await screen.findByText('the cluster said no')).toBeInTheDocument();
  });
  it('does not read the diff again just because the list refreshed', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const calls = route({ '/api/rollout/diff': difference, '/api/rollout': answer });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Diff' }));
    await waitFor(() => {
      expect(screen.getByTestId('diff')).toBeInTheDocument();
    });
    const before = calls.filter((one) => one.includes('/api/rollout/diff')).length;

    await act(async () => {
      await vi.advanceTimersByTimeAsync(40000);
    });

    const after = calls.filter((one) => one.includes('/api/rollout/diff')).length;
    expect(after).toBe(before);
    vi.useRealTimers();
  });

  it('says so when the revision list itself could not be read', async () => {
    route({}, { '/api/rollout': 500 });

    render(<RolloutRevisions target={target} onDone={noop} />);

    expect(await screen.findByText('the cluster said no')).toBeInTheDocument();
  });

  it('says the workload has rolled out nothing it can still read', async () => {
    route({ '/api/rollout': { supported: true, revisions: [] } });

    render(<RolloutRevisions target={target} onDone={noop} />);

    expect(await screen.findByText(/rolled out nothing/)).toBeInTheDocument();
  });

  it('calls the age unknown when the cluster never said when', async () => {
    route({
      '/api/rollout': {
        supported: true,
        revisions: [
          { number: 3, name: 'web-abc', current: true, images: [] },
          { number: 2, name: 'web-def', images: [] },
        ],
      },
    });

    render(<RolloutRevisions target={target} onDone={noop} />);

    expect(await screen.findAllByText('unknown')).not.toHaveLength(0);
  });

  it('asks for the name before going back on a protected cluster', async () => {
    const calls = route({ '/api/rollout': answer, '/api/action': { message: 'went back to 2' } });
    protectedCluster();
    stubDialog();

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Go back to this' }));

    expect(await screen.findByText(/Put web back to revision 2/)).toBeInTheDocument();
    expect(calls.some((one) => one.includes('/api/action'))).toBe(false);
  });

  it('goes back once the name has been typed', async () => {
    const calls = route({ '/api/rollout': answer, '/api/action': { message: 'went back to 2' } });
    protectedCluster();
    stubDialog();

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Go back to this' }));
    await userEvent.type(await screen.findByLabelText('Name'), 'web');
    await userEvent.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      const undo = calls.find((one) => one.includes('/api/action'));
      expect(undo).toContain('confirm=web');
    });
  });

  it('leaves the workload alone when the name is not confirmed', async () => {
    const calls = route({ '/api/rollout': answer });
    protectedCluster();
    stubDialog();

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Go back to this' }));
    await userEvent.click(await screen.findByRole('button', { name: 'Cancel' }));

    await waitFor(() => {
      expect(screen.queryByText(/Put web back to revision 2/)).not.toBeInTheDocument();
    });
    expect(calls.some((one) => one.includes('/api/action'))).toBe(false);
  });

  it('says what stopped the way back rather than a silent nothing', async () => {
    route({ '/api/rollout': answer }, { '/api/action': 403 });

    render(<RolloutRevisions target={target} onDone={noop} />);
    await screen.findByText('#2');
    await userEvent.click(screen.getByRole('button', { name: 'Go back to this' }));

    await waitFor(() => {
      expect(useToastsStore.getState().toasts.some((one) => one.message.includes('undo web'))).toBe(
        true,
      );
    });
  });
});

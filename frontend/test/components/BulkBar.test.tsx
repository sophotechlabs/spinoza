import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import BulkBar from '../../src/components/BulkBar';
import type { ObjectRef } from '../../src/lib/types';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';

function podRef(name: string): ObjectRef {
  return { group: '', version: 'v1', resource: 'pods', namespace: 'prod', name };
}

function deploymentRef(name: string): ObjectRef {
  return { group: 'apps', version: 'v1', resource: 'deployments', namespace: 'prod', name };
}

function renderBar(targets: ObjectRef[], kind = 'Pod') {
  const onDone = vi.fn();
  const onClear = vi.fn();
  const view = render(<BulkBar kind={kind} targets={targets} onDone={onDone} onClear={onClear} />);
  return { onDone, onClear, view };
}

function protectCluster() {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [],
    protection: 'protected',
  });
}

const ALLOWED = { refused: [] };

function noop() {
  return undefined;
}

function ok(body: unknown) {
  return Promise.resolve({ ok: true, json: () => Promise.resolve(body) });
}

function asked(access: unknown, onObject: (url: string) => unknown = () => ok({})) {
  return vi.fn((url: string, init?: RequestInit) => {
    void init;
    if (url === '/api/access') {
      return ok(access);
    }
    return onObject(url);
  });
}

function stub(access: unknown, onObject?: (url: string) => unknown) {
  const fetchMock = asked(access, onObject);
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function objectCalls(fetchMock: ReturnType<typeof asked>): string[] {
  return fetchMock.mock.calls.map((call) => call[0]).filter((url) => url !== '/api/access');
}

function accessBody(fetchMock: ReturnType<typeof asked>): unknown {
  const call = fetchMock.mock.calls.find((one) => one[0] === '/api/access');
  const init = call?.[1];
  if (typeof init?.body !== 'string') {
    throw new Error('the access question carried no body');
  }
  return JSON.parse(init.body);
}

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});
const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

beforeEach(() => {
  useToastsStore.getState().clear();
  showModal.mockClear();
  close.mockClear();
  HTMLDialogElement.prototype.showModal = showModal;
  HTMLDialogElement.prototype.close = close;
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('BulkBar', () => {
  it('stays out of the way with nothing selected', () => {
    const { view } = renderBar([]);

    expect(view.container).toBeEmptyDOMElement();
  });

  it('counts one selected object in the singular', () => {
    renderBar([podRef('web-0')]);

    expect(screen.getByRole('status')).toHaveTextContent('1 Pod selected');
  });

  it('counts several', () => {
    renderBar([podRef('web-0'), podRef('web-1')]);

    expect(screen.getByRole('status')).toHaveTextContent('2 Pod objects selected');
  });

  it('offers Restart only for a kind that can be restarted', () => {
    renderBar([podRef('web-0')]);
    expect(screen.queryByRole('button', { name: 'Restart' })).not.toBeInTheDocument();

    renderBar([deploymentRef('web')], 'Deployment');
    expect(screen.getByRole('button', { name: 'Restart' })).toBeInTheDocument();
  });

  it('asks before deleting', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(ALLOWED);
    renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(await screen.findByText('Delete 1 object?')).toBeInTheDocument();
    expect(objectCalls(fetchMock)).toEqual([]);
  });

  it('backs out of a delete', async () => {
    const user = userEvent.setup();
    stub(ALLOWED);
    renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('deletes every selected object and says how many', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(ALLOWED);
    const { onDone } = renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(await screen.findByRole('button', { name: 'Confirm' }));

    expect(objectCalls(fetchMock)).toHaveLength(2);
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Deleted 2' }),
    ]);
    expect(onDone).toHaveBeenCalledTimes(1);
  });

  it('names the ones that failed', async () => {
    const user = userEvent.setup();
    stub(ALLOWED, (url) => {
      if (url.includes('web-1')) {
        return Promise.resolve({ ok: false, status: 500, json: () => Promise.resolve({}) });
      }
      return ok({});
    });
    renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(await screen.findByRole('button', { name: 'Confirm' }));

    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'error', message: 'Deleted 1, 1 failed: web-1' }),
    ]);
  });

  it('restarts every selected workload', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(ALLOWED, () => ok({ action: 'restart' }));
    renderBar([deploymentRef('web'), deploymentRef('api')], 'Deployment');

    await user.click(screen.getByRole('button', { name: 'Restart' }));
    await user.click(await screen.findByRole('button', { name: 'Confirm' }));

    expect(objectCalls(fetchMock)).toHaveLength(2);
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Restarted 2' }),
    ]);
  });

  it('hands the selection back when asked to clear it', async () => {
    const user = userEvent.setup();
    const { onClear } = renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Clear selection' }));

    expect(onClear).toHaveBeenCalledTimes(1);
  });
});

describe('BulkBar asking what the cluster allows', () => {
  it('asks about the whole selection, by name', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(ALLOWED);
    renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await waitFor(() => {
      expect(accessBody(fetchMock)).toEqual({
        capability: 'delete',
        refs: [podRef('web-0'), podRef('web-1')],
      });
    });
  });

  it('asks about restarting when that is what was clicked', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(ALLOWED);
    renderBar([deploymentRef('web')], 'Deployment');

    await user.click(screen.getByRole('button', { name: 'Restart' }));

    await waitFor(() => {
      expect(accessBody(fetchMock)).toEqual({
        capability: 'restart',
        refs: [deploymentRef('web')],
      });
    });
  });

  it('says it is checking, and offers nothing to confirm, until the answer lands', async () => {
    const user = userEvent.setup();
    let answer: (body: unknown) => void = noop;
    const held = new Promise((resolve) => {
      answer = resolve;
    });
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url === '/api/access') {
          return held;
        }
        return ok({});
      }),
    );
    renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(screen.getByText('Checking what the cluster allows…')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument();
    answer({ ok: true, json: () => Promise.resolve(ALLOWED) });
    expect(await screen.findByRole('button', { name: 'Confirm' })).toBeInTheDocument();
  });

  it('names the rows that will be refused and still lets the rest through', async () => {
    const user = userEvent.setup();
    stub({ refused: [{ at: 1, reason: 'you may not delete pods here' }] });
    renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(
      await screen.findByText(
        'Delete 2 objects? web-1 will be refused: you may not delete pods here',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
  });

  it('counts them instead of naming them when there are more than a few', async () => {
    const user = userEvent.setup();
    stub({
      refused: [0, 1, 2, 3].map((at) => ({ at, reason: 'no' })),
    });
    renderBar([0, 1, 2, 3, 4].map((i) => podRef(`web-${String(i)}`)));

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(
      await screen.findByText('Delete 5 objects? 4 of 5 will be refused: no'),
    ).toBeInTheDocument();
  });

  it('does not offer to act when every row is refused', async () => {
    const user = userEvent.setup();
    stub({
      refused: [
        { at: 0, reason: 'no deleting pods' },
        { at: 1, reason: 'no deleting pods' },
      ],
    });
    renderBar([podRef('web-0'), podRef('web-1')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(
      await screen.findByText('The cluster refuses all 2: no deleting pods'),
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
  });

  it('ignores a refusal that names no row in the selection', async () => {
    const user = userEvent.setup();
    stub({
      refused: [
        { at: 7, reason: 'about nothing here' },
        { at: -1, reason: 'about nothing here' },
      ],
    });
    renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(await screen.findByText('Delete 1 object?')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
  });

  it('stops nothing when the question cannot be put', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url === '/api/access') {
          return Promise.reject(new Error('the backend did not answer in time'));
        }
        return ok({});
      }),
    );
    renderBar([podRef('web-0')]);

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(await screen.findByRole('button', { name: 'Confirm' })).toBeInTheDocument();
    expect(screen.getByText('Delete 1 object?')).toBeInTheDocument();
  });

  it('drops the question when the selection changes underneath it', async () => {
    const user = userEvent.setup();
    stub(ALLOWED);
    const { view } = renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await screen.findByRole('button', { name: 'Confirm' });

    view.rerender(
      <BulkBar kind="Pod" targets={[podRef('api-0')]} onDone={vi.fn()} onClear={vi.fn()} />,
    );

    expect(screen.queryByRole('button', { name: 'Confirm' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('never shows an answer about a selection that has been left behind', async () => {
    const user = userEvent.setup();
    const answers: ((body: unknown) => void)[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url === '/api/access') {
          return new Promise((resolve) => {
            answers.push(resolve);
          });
        }
        return ok({});
      }),
    );
    const { view } = renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    view.rerender(
      <BulkBar kind="Pod" targets={[podRef('api-0')]} onDone={vi.fn()} onClear={vi.fn()} />,
    );
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    answers[1]({ ok: true, json: () => Promise.resolve(ALLOWED) });
    await screen.findByRole('button', { name: 'Confirm' });
    answers[0]({
      ok: true,
      json: () => Promise.resolve({ refused: [{ at: 0, reason: 'about the old selection' }] }),
    });

    await waitFor(() => {
      expect(screen.getByText('Delete 1 object?')).toBeInTheDocument();
    });
    expect(screen.queryByText(/about the old selection/)).not.toBeInTheDocument();
  });
});

describe('BulkBar on a protected cluster', () => {
  it('asks for the cluster name before deleting', async () => {
    const user = userEvent.setup();
    protectCluster();
    stub(ALLOWED);

    renderBar([podRef('web-0'), podRef('web-1')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(
      await screen.findByText(
        'Deleting 2 objects on p-mk1 in one go. This asks for the cluster name, not an object name.',
      ),
    ).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  });

  it('carries what the cluster refuses into the typed question', async () => {
    const user = userEvent.setup();
    protectCluster();
    stub({ refused: [{ at: 0, reason: 'no deleting web-0' }] });

    renderBar([podRef('web-0'), podRef('web-1')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(await screen.findByText(/web-0 will be refused: no deleting web-0/)).toBeInTheDocument();
  });

  it('does not ask for the name when nothing can be deleted anyway', async () => {
    const user = userEvent.setup();
    protectCluster();
    stub({ refused: [{ at: 0, reason: 'no deleting pods' }] });

    renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(
      await screen.findByText('The cluster refuses all 1: no deleting pods'),
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
  });

  it('sends each object name once the cluster name is typed', async () => {
    const user = userEvent.setup();
    protectCluster();
    const fetchMock = stub(ALLOWED);

    const { onDone } = renderBar([podRef('web-0'), podRef('web-1')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.type(await screen.findByLabelText('Name'), 'p-mk1');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onDone).toHaveBeenCalled();
    });
    const calls = objectCalls(fetchMock);
    expect(calls[0]).toContain('confirm=web-0');
    expect(calls[1]).toContain('confirm=web-1');
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    protectCluster();
    stub(ALLOWED);

    renderBar([podRef('web-0')]);
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(await screen.findByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
  });

  it('still restarts with one click', async () => {
    const user = userEvent.setup();
    protectCluster();
    const fetchMock = stub(ALLOWED, () => ok({ action: 'restart', message: 'ok' }));

    const { onDone } = renderBar([deploymentRef('web')], 'Deployment');
    await user.click(screen.getByRole('button', { name: 'Restart' }));
    await user.click(await screen.findByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      expect(onDone).toHaveBeenCalled();
    });
    expect(objectCalls(fetchMock)[0]).not.toContain('confirm');
  });
});

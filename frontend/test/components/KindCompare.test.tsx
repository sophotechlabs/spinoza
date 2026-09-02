import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ResourceDescriptor } from '../../src/lib/types';
import KindCompare from '../../src/components/KindCompare';
import { summaryOf } from '../../src/lib/compare';

const kind: ResourceDescriptor = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  kind: 'Deployment',
  namespaced: true,
  category: 'Workloads',
};

const target = { kubeconfig: '', name: 'p-mk2', namespace: 'flux-system', object: '' };

const answer = {
  resource: 'deployments',
  leftContext: 'p-mk1',
  rightContext: 'p-mk2',
  namespace: 'flux-system',
  same: 1,
  differs: 1,
  onlyHere: 1,
  onlyThere: 1,
  objects: [
    { namespace: 'flux-system', name: 'source-controller', verdict: 'same' },
    { namespace: 'flux-system', name: 'flux-operator', verdict: 'differs', lines: 4 },
    { namespace: 'flux-system', name: 'only-here', verdict: 'onlyHere' },
    { namespace: 'flux-system', name: 'only-there', verdict: 'onlyThere' },
  ],
};

function stub(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn((url: string) => {
    void url;
    return Promise.resolve({ ok, status, json: () => Promise.resolve(body) });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function renderPanel(onOpen = vi.fn()) {
  render(<KindCompare kind={kind} namespace="flux-system" target={target} onOpen={onOpen} />);
  return onOpen;
}

async function compare(user: ReturnType<typeof userEvent.setup>) {
  await user.click(screen.getByRole('button', { name: 'Compare every Deployment' }));
  await screen.findByText(/p-mk1 against p-mk2/);
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('comparing a whole kind', () => {
  it('waits to be asked before reading anything', () => {
    const fetchMock = stub(answer);

    renderPanel();

    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole('button', { name: 'Compare every Deployment' })).toBeInTheDocument();
  });

  it('names the kind it would compare', () => {
    stub(answer);

    renderPanel();

    expect(screen.getByRole('button', { name: 'Compare every Deployment' })).toBeInTheDocument();
  });

  it('asks the backend for the kind and the context', async () => {
    const user = userEvent.setup();
    const fetchMock = stub(answer);
    renderPanel();

    await compare(user);

    const url = fetchMock.mock.calls[0][0];
    expect(url).toContain('/api/compare/kind');
    expect(url).toContain('resource=deployments');
    expect(url).toContain('namespace=flux-system');
    expect(url).toContain('against=p-mk2');
  });

  it('counts each verdict in the summary', async () => {
    const user = userEvent.setup();
    stub(answer);
    renderPanel();

    await compare(user);

    expect(
      screen.getByText(/1 same · 1 differ · 1 only on p-mk1 · 1 only on p-mk2/),
    ).toBeInTheDocument();
  });

  it('shows only what drifted until asked for everything', async () => {
    const user = userEvent.setup();
    stub(answer);
    renderPanel();
    await compare(user);

    expect(screen.queryByText('source-controller')).not.toBeInTheDocument();

    await user.click(screen.getByLabelText('Only what differs'));

    expect(screen.getByText('source-controller')).toBeInTheDocument();
  });

  it('says how far apart a differing pair is', async () => {
    const user = userEvent.setup();
    stub(answer);
    renderPanel();

    await compare(user);

    expect(screen.getByText('4 lines')).toBeInTheDocument();
  });

  it('opens the object that was clicked', async () => {
    const user = userEvent.setup();
    stub(answer);
    const onOpen = renderPanel();
    await compare(user);

    await user.click(screen.getByRole('button', { name: 'flux-operator' }));

    expect(onOpen).toHaveBeenCalledWith({
      group: 'apps',
      version: 'v1',
      resource: 'deployments',
      namespace: 'flux-system',
      name: 'flux-operator',
    });
  });

  it('will not open an object this cluster does not have', async () => {
    const user = userEvent.setup();
    stub(answer);
    renderPanel();

    await compare(user);

    expect(screen.getByRole('button', { name: 'only-there' })).toBeDisabled();
  });

  it('falls back to the panel namespace when an object carries none', async () => {
    const user = userEvent.setup();
    stub({
      ...answer,
      objects: [{ name: 'cluster-admin', verdict: 'differs', lines: 2 }],
      same: 0,
      differs: 1,
      onlyHere: 0,
      onlyThere: 0,
    });
    const onOpen = renderPanel();
    await compare(user);

    await user.click(screen.getByRole('button', { name: 'cluster-admin' }));

    expect(onOpen).toHaveBeenCalledWith(expect.objectContaining({ namespace: 'flux-system' }));
  });

  it('says when the two clusters agree on everything', async () => {
    const user = userEvent.setup();
    stub({
      ...answer,
      objects: [{ namespace: 'flux-system', name: 'source-controller', verdict: 'same' }],
      same: 1,
      differs: 0,
      onlyHere: 0,
      onlyThere: 0,
    });
    renderPanel();

    await compare(user);

    expect(screen.getByText('Every one of them matches.')).toBeInTheDocument();
  });

  it('says when neither cluster has any', async () => {
    const user = userEvent.setup();
    stub({ ...answer, objects: [], same: 0, differs: 0, onlyHere: 0, onlyThere: 0 });
    renderPanel();

    await compare(user);

    expect(screen.getByText('Neither cluster has one of these.')).toBeInTheDocument();
  });

  it('says there is nothing to show when everything is filtered out by hand', async () => {
    const user = userEvent.setup();
    stub({
      ...answer,
      objects: [{ namespace: 'flux-system', name: 'source-controller', verdict: 'same' }],
      same: 1,
      differs: 0,
      onlyHere: 0,
      onlyThere: 0,
    });
    renderPanel();
    await compare(user);

    await user.click(screen.getByLabelText('Only what differs'));

    expect(screen.getByText('source-controller')).toBeInTheDocument();
  });

  it('says when the comparison itself failed', async () => {
    const user = userEvent.setup();
    stub({ message: 'the other cluster refused the list' }, false, 500);
    renderPanel();

    await user.click(screen.getByRole('button', { name: 'Compare every Deployment' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('refused the list');
  });

  it('falls back to a plain reason when the failure carries none', async () => {
    const user = userEvent.setup();
    const rejectNonError = vi.fn<() => Promise<never>>().mockRejectedValue('nope');
    vi.stubGlobal('fetch', rejectNonError);
    renderPanel();

    await user.click(screen.getByRole('button', { name: 'Compare every Deployment' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('the comparison did not run');
  });

  it('says it is reading while it waits', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    );
    renderPanel();

    await user.click(screen.getByRole('button', { name: 'Compare every Deployment' }));

    await waitFor(() => {
      expect(screen.getByText('reading both clusters')).toBeInTheDocument();
    });
  });

  it('says when the two sides were matched by name alone', async () => {
    const user = userEvent.setup();
    stub({ ...answer, matchedByName: true });
    renderPanel();

    await compare(user);

    expect(screen.getByText(/matched by name across namespaces/)).toBeInTheDocument();
  });

  it('drops a completed comparison when its target changes', async () => {
    const user = userEvent.setup();
    stub(answer);
    const view = render(
      <KindCompare kind={kind} namespace="flux-system" target={target} onOpen={vi.fn()} />,
    );
    await compare(user);

    view.rerender(
      <KindCompare
        kind={kind}
        namespace="flux-system"
        target={{ ...target, name: 'p-mk3' }}
        onOpen={vi.fn()}
      />,
    );

    expect(screen.queryByText(/p-mk1 against p-mk2/)).not.toBeInTheDocument();
  });

  it('ignores a comparison that completes after its target changes', async () => {
    const user = userEvent.setup();
    let finish: (response: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((resolve) => {
            finish = resolve;
          }),
      ),
    );
    const view = render(
      <KindCompare kind={kind} namespace="flux-system" target={target} onOpen={vi.fn()} />,
    );
    await user.click(screen.getByRole('button', { name: 'Compare every Deployment' }));

    view.rerender(
      <KindCompare
        kind={kind}
        namespace="flux-system"
        target={{ ...target, name: 'p-mk3' }}
        onOpen={vi.fn()}
      />,
    );
    await act(async () => {
      finish({ ok: true, status: 200, json: () => Promise.resolve(answer) });
      await Promise.resolve();
    });

    expect(screen.queryByText(/p-mk1 against p-mk2/)).not.toBeInTheDocument();
    expect(screen.queryByText('reading both clusters')).not.toBeInTheDocument();
  });
});

describe('the summary line', () => {
  it('leaves out the sides that hold nothing of their own', () => {
    const line = summaryOf({
      resource: 'deployments',
      leftContext: 'p-mk1',
      rightContext: 'p-mk2',
      objects: [],
      same: 4,
      differs: 2,
      onlyHere: 0,
      onlyThere: 0,
    });

    expect(line).toBe('4 same · 2 differ');
  });
});

describe('a difference nobody could measure', () => {
  it('says it differs without claiming a line count', async () => {
    const user = userEvent.setup();
    stub({
      ...answer,
      objects: [{ namespace: 'flux-system', name: 'web', verdict: 'differs' }],
      same: 0,
      differs: 1,
      onlyHere: 0,
      onlyThere: 0,
    });
    renderPanel();

    await compare(user);

    expect(screen.getByText('differs')).toBeInTheDocument();
    expect(screen.queryByText(/lines/)).not.toBeInTheDocument();
  });
});

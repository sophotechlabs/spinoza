import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('../../src/components/YamlDiff', () => ({
  default: ({ left, right, sideBySide }: { left: string; right: string; sideBySide: boolean }) => (
    <div data-testid="yaml-diff" data-side-by-side={String(sideBySide)}>
      {left}|{right}
    </div>
  ),
}));

import ComparePanel from '../../src/components/ComparePanel';
import type { ObjectRef } from '../../src/lib/types';
import { useContextsStore } from '../../src/store/contexts';

const target: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'prod',
  name: 'web',
};

const answer = {
  left: 'spec:\n  replicas: 3\n',
  right: 'spec:\n  replicas: 5\n',
  leftContext: 'p-mk1',
  rightContext: 'gke-prod',
  identical: false,
};

function withContexts(names: string[]) {
  useContextsStore.getState().setList({
    current: { kubeconfig: '', name: 'p-mk1' },
    kubeconfigs: [
      {
        label: 'default',
        path: '',
        removable: false,
        contexts: names.map((name) => ({ name, cluster: name })),
      },
    ],
    protection: 'open',
  });
}

function stub(body: unknown, ok = true) {
  const mock = vi
    .fn()
    .mockResolvedValue({ ok, status: ok ? 200 : 404, json: () => Promise.resolve(body) });
  vi.stubGlobal('fetch', mock);
  return mock;
}

beforeEach(() => {
  withContexts(['p-mk1', 'gke-prod']);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('what the panel shows before anything is chosen', () => {
  it('asks for an object or a kind when neither is there', () => {
    render(<ComparePanel target={null} kind={null} namespace="" onOpen={vi.fn()} />);

    expect(screen.getByText('Open a kind, or select an object, to compare.')).toBeInTheDocument();
  });

  it('says so when there is nothing to compare against', () => {
    withContexts(['p-mk1']);

    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    expect(screen.getByText(/needs a second context/)).toBeInTheDocument();
  });

  it('offers the other contexts, never the current one', () => {
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    expect(screen.getByRole('option', { name: 'gke-prod' })).toBeInTheDocument();
    expect(screen.queryByRole('option', { name: 'p-mk1' })).not.toBeInTheDocument();
  });

  it('starts on the namespace the object is in', () => {
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    expect(screen.getByLabelText('Namespace')).toHaveValue('prod');
  });

  it('keeps the button disabled until a context is picked', () => {
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    expect(screen.getByRole('button', { name: 'Compare' })).toBeDisabled();
  });
});

describe('running a comparison', () => {
  it('reports what differs and shows the diff', async () => {
    const user = userEvent.setup();
    stub(answer);
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    expect(await screen.findByTestId('yaml-diff')).toHaveTextContent('replicas: 3');
    expect(screen.getByText(/p-mk1 against gke-prod/)).toBeInTheDocument();
    expect(screen.getByText(/2 lines differ · spec/)).toBeInTheDocument();
  });

  it('says when the two sides match', async () => {
    const user = userEvent.setup();
    stub({ ...answer, right: answer.left, identical: true });
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    expect(
      await screen.findByText(/identical once the per-cluster fields are stripped/),
    ).toBeInTheDocument();
  });

  it('passes the far namespace when it was changed', async () => {
    const user = userEvent.setup();
    const mock = stub(answer);
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.clear(screen.getByLabelText('Namespace'));
    await user.type(screen.getByLabelText('Namespace'), 'staging');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    await waitFor(() => {
      expect(mock.mock.calls[0][0]).toContain('againstNamespace=staging');
    });
  });

  it('asks for everything when the box is ticked', async () => {
    const user = userEvent.setup();
    const mock = stub(answer);
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByLabelText('Show everything'));
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    await waitFor(() => {
      expect(mock.mock.calls[0][0]).toContain('raw=true');
    });
  });

  it('says plainly when the other cluster has no such object', async () => {
    const user = userEvent.setup();
    stub({ ...answer, right: '', missing: 'that context has no such object' });
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    expect(await screen.findByText('that context has no such object')).toBeInTheDocument();
    expect(screen.queryByTestId('yaml-diff')).not.toBeInTheDocument();
  });

  it('reports a request that failed', async () => {
    const user = userEvent.setup();
    stub({ message: 'the apiserver is unreachable' }, false);
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    expect(await screen.findByText('the apiserver is unreachable')).toBeInTheDocument();
  });

  it('drops the result when another object is selected', async () => {
    const user = userEvent.setup();
    stub(answer);
    const view = render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);
    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));
    await screen.findByTestId('yaml-diff');

    view.rerender(
      <ComparePanel
        target={{ ...target, name: 'api' }}
        kind={null}
        namespace=""
        onOpen={vi.fn()}
      />,
    );

    expect(screen.queryByTestId('yaml-diff')).not.toBeInTheDocument();
  });
});

describe('the odd cases the summary and errors have to cover', () => {
  it('counts lines even when no section can be named', async () => {
    const user = userEvent.setup();
    stub({ ...answer, left: '  replicas: 3\n', right: '  replicas: 5\n' });
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    expect(await screen.findByText(/2 lines differ$/)).toBeInTheDocument();
  });

  it('falls back to a plain sentence when the failure is not an Error', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    render(<ComparePanel target={target} kind={null} namespace="" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');
    await user.click(screen.getByRole('button', { name: 'Compare' }));

    expect(await screen.findByText('the comparison did not run')).toBeInTheDocument();
  });
});

describe('choosing what to compare', () => {
  const kind = {
    group: 'apps',
    version: 'v1',
    resource: 'deployments',
    kind: 'Deployment',
    namespaced: true,
    category: 'Workloads',
  };

  it('opens on the kind when nothing is selected', () => {
    stub(answer);

    render(<ComparePanel target={null} kind={kind} namespace="prod" onOpen={vi.fn()} />);

    expect(screen.getByLabelText('What to compare')).toHaveValue('kind');
    expect(screen.getByRole('option', { name: 'this object' })).toBeDisabled();
  });

  it('opens on the object when one is selected', () => {
    stub(answer);

    render(<ComparePanel target={target} kind={kind} namespace="prod" onOpen={vi.fn()} />);

    expect(screen.getByLabelText('What to compare')).toHaveValue('object');
  });

  it('asks for a context before it reads a whole kind', () => {
    stub(answer);

    render(<ComparePanel target={null} kind={kind} namespace="prod" onOpen={vi.fn()} />);

    expect(screen.getByText('Pick a context to compare against.')).toBeInTheDocument();
  });

  it('offers the kind comparison once a context is picked', async () => {
    const user = userEvent.setup();
    stub(answer);
    render(<ComparePanel target={null} kind={kind} namespace="prod" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');

    expect(
      await screen.findByRole('button', { name: 'Compare every Deployment' }),
    ).toBeInTheDocument();
  });

  it('keeps the context it was given when the object changes', async () => {
    const user = userEvent.setup();
    stub(answer);
    const view = render(
      <ComparePanel target={target} kind={kind} namespace="prod" onOpen={vi.fn()} />,
    );
    await user.selectOptions(screen.getByLabelText('Against'), 'gke-prod');

    view.rerender(
      <ComparePanel
        target={{ ...target, name: 'api' }}
        kind={kind}
        namespace="prod"
        onOpen={vi.fn()}
      />,
    );

    const kept = screen.getByLabelText('Against');
    expect(kept).toHaveDisplayValue('gke-prod');
  });

  it('switches back to the object once one is chosen', async () => {
    const user = userEvent.setup();
    stub(answer);
    render(<ComparePanel target={target} kind={kind} namespace="prod" onOpen={vi.fn()} />);

    await user.selectOptions(screen.getByLabelText('What to compare'), 'kind');
    await user.selectOptions(screen.getByLabelText('What to compare'), 'object');

    expect(screen.getByRole('button', { name: 'Compare' })).toBeInTheDocument();
  });
});

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

interface EditorStubProps {
  value: string;
  onChange: (value: string | undefined) => void;
  options: { readOnly: boolean };
}

vi.mock('../../src/lib/monaco', () => ({ defineEditorTheme: vi.fn() }));

vi.mock('@monaco-editor/react', () => ({
  default: ({ value, onChange, options }: EditorStubProps) => (
    <textarea
      aria-label="yaml"
      readOnly={options.readOnly}
      value={value}
      onChange={(event) => {
        onChange(event.target.value);
      }}
    />
  ),
}));

import InspectYaml from '../../src/components/InspectYaml';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';
import { accessKey, useAccessStore } from '../../src/store/access';
import { EMPTY_CONTEXTS } from '../../src/store/contexts';
import { hasUnsaved, setUnsaved } from '../../src/lib/unsaved';
import type { ObjectDetail, ObjectRef } from '../../src/lib/types';

const target: ObjectRef = {
  group: 'apps',
  version: 'v1',
  resource: 'deployments',
  namespace: 'flux-system',
  name: 'web',
};

const YAML = 'kind: Deployment\n';

function detailFor(yaml: string): ObjectDetail {
  return {
    apiVersion: 'apps/v1',
    kind: 'Deployment',
    name: 'web',
    namespace: 'flux-system',
    uid: 'uid-web',
    createdAt: '2026-07-27T09:00:00Z',
    yaml,
  };
}

function renderYaml(yaml = YAML, detail?: ObjectDetail) {
  const onApplied = vi.fn();
  const onDeleted = vi.fn();
  const view = render(
    <InspectYaml
      target={target}
      detail={detail ?? detailFor(yaml)}
      onApplied={onApplied}
      onDeleted={onDeleted}
    />,
  );
  return { onApplied, onDeleted, view };
}

const ownedBy = {
  controller: 'argocd',
  kind: 'Application',
  ref: {
    group: 'argoproj.io',
    version: 'v1alpha1',
    resource: 'applications',
    namespace: 'argocd',
    name: 'podinfo',
  },
};

function okResponse(payload: unknown) {
  return { ok: true, json: () => Promise.resolve(payload) };
}

function errorResponse(status: number, message: string) {
  return { ok: false, status, json: () => Promise.resolve({ message }) };
}

describe('InspectYaml', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(okResponse({})));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('starts clean with the actions disabled', async () => {
    renderYaml();

    expect(await screen.findByLabelText('yaml')).toHaveValue(YAML);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Revert' })).toBeDisabled();
    expect(screen.queryByText('unsaved changes')).not.toBeInTheDocument();
  });

  it('enables the actions once the draft diverges', async () => {
    const user = userEvent.setup();
    renderYaml();

    await user.type(await screen.findByLabelText('yaml'), 'x');

    expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled();
    expect(screen.getByText('unsaved changes')).toBeInTheDocument();
  });

  it('reverts the draft', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Revert' }));

    expect(screen.getByLabelText('yaml')).toHaveValue(YAML);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('applies the draft and reports success', async () => {
    const user = userEvent.setup();
    const { onApplied } = renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(await screen.findByText('Applied.')).toBeInTheDocument();
    expect(onApplied).toHaveBeenCalledTimes(1);
  });

  it('surfaces an apply failure', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(errorResponse(409, 'the object has been modified')),
    );
    const { onApplied } = renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(await screen.findByText('the object has been modified')).toBeInTheDocument();
    expect(onApplied).not.toHaveBeenCalled();
  });

  it('falls back to a generic apply message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(await screen.findByText('apply failed')).toBeInTheDocument();
  });

  it('asks for confirmation before deleting', async () => {
    const user = userEvent.setup();
    const { onDeleted } = renderYaml();

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(screen.getByText('Delete web?')).toBeInTheDocument();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it('cancels a pending delete', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
    expect(screen.queryByText('Delete web?')).not.toBeInTheDocument();
  });

  it('deletes on confirmation and says so out loud', async () => {
    const user = userEvent.setup();
    useToastsStore.getState().clear();
    const { onDeleted } = renderYaml();
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(onDeleted).toHaveBeenCalledTimes(1);
    expect(useToastsStore.getState().toasts).toEqual([
      expect.objectContaining({ tone: 'ok', message: 'Deleted Deployment web' }),
    ]);
  });

  it('surfaces a delete failure', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(errorResponse(403, 'forbidden')));
    const { onDeleted } = renderYaml();
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('forbidden')).toBeInTheDocument();
    expect(onDeleted).not.toHaveBeenCalled();
  });

  it('falls back to a generic delete message for a non-Error rejection', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderYaml();
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(await screen.findByText('delete failed')).toBeInTheDocument();
  });

  it('reseeds an untouched draft when the object is refetched', async () => {
    const { view } = renderYaml();
    await screen.findByLabelText('yaml');

    const next = 'kind: Deployment\nreplicas: 3\n';
    view.rerender(
      <InspectYaml
        target={target}
        detail={detailFor(next)}
        onApplied={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('yaml')).toHaveValue(next);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('keeps an edited draft when the object changes underneath', async () => {
    const user = userEvent.setup();
    const { view } = renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');
    const mine = screen.getByLabelText<HTMLTextAreaElement>('yaml').value;

    view.rerender(
      <InspectYaml
        target={target}
        detail={detailFor('kind: Deployment\nreplicas: 3\n')}
        onApplied={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('yaml')).toHaveValue(mine);
    expect(screen.getByText('changed on the server, Revert to load it')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled();
  });

  it('loads the server version when a stale draft is reverted', async () => {
    const user = userEvent.setup();
    const { view } = renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');

    const next = 'kind: Deployment\nreplicas: 3\n';
    view.rerender(
      <InspectYaml
        target={target}
        detail={detailFor(next)}
        onApplied={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );
    await user.click(screen.getByRole('button', { name: 'Revert' }));

    expect(screen.getByLabelText('yaml')).toHaveValue(next);
    expect(screen.queryByText('changed on the server, Revert to load it')).not.toBeInTheDocument();
  });

  it('drops an edited draft when a different object is selected', async () => {
    const user = userEvent.setup();
    const { view } = renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');

    const next = 'kind: Deployment\nreplicas: 3\n';
    view.rerender(
      <InspectYaml
        target={{ ...target, name: 'other' }}
        detail={detailFor(next)}
        onApplied={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );

    expect(screen.getByLabelText('yaml')).toHaveValue(next);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('settles after a successful apply is echoed back', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(okResponse({})));
    const { view } = renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');
    const applied = screen.getByLabelText<HTMLTextAreaElement>('yaml').value;
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    view.rerender(
      <InspectYaml
        target={target}
        detail={detailFor(applied)}
        onApplied={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );

    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
    expect(screen.queryByText('changed on the server, Revert to load it')).not.toBeInTheDocument();
  });
});

describe('leaving with an unsaved draft', () => {
  afterEach(() => {
    setUnsaved(false);
  });

  it('tells the rest of the app the editor is dirty', async () => {
    const user = userEvent.setup();
    renderYaml();
    expect(hasUnsaved()).toBe(false);

    await user.type(await screen.findByLabelText('yaml'), 'x');

    expect(hasUnsaved()).toBe(true);
  });

  it('goes quiet again once the draft is reverted', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Revert' }));

    expect(hasUnsaved()).toBe(false);
  });

  it('goes quiet when the editor goes away', async () => {
    const user = userEvent.setup();
    const { view } = renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    view.unmount();

    expect(hasUnsaved()).toBe(false);
  });

  it('asks the browser to confirm a reload while dirty', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    const event = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
  });

  it('lets a reload through once the draft is saved', () => {
    renderYaml();

    const event = new Event('beforeunload', { cancelable: true });
    window.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
  });
});

describe('copying the manifest', () => {
  it('copies the draft as it stands', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Copy YAML' }));

    expect(writeText).toHaveBeenCalledWith(`${YAML}x`);
  });
});

describe('announcing what an action did', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(okResponse({})));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('puts the apply result in a live region', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(await screen.findByRole('status')).toHaveTextContent('Applied.');
  });

  it('puts an apply failure in an assertive one', async () => {
    const user = userEvent.setup();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(errorResponse(500, 'the api server said no')));
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('the api server said no');
  });
});

describe('the inline delete confirm', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(okResponse({})));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('moves focus onto Confirm and back to Delete on cancel', async () => {
    const user = userEvent.setup();
    renderYaml();
    await screen.findByLabelText('yaml');

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Confirm' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'Delete' }));
  });
});

describe('InspectYaml on a protected cluster', () => {
  const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
    this.open = true;
  });
  const close = vi.fn(function close(this: HTMLDialogElement) {
    this.open = false;
  });

  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(okResponse({})));
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

  afterEach(() => {
    vi.unstubAllGlobals();
    useToastsStore.getState().clear();
  });

  it('asks for the name after the ownership warning is acknowledged', async () => {
    const user = userEvent.setup();
    renderYaml(YAML, { ...detailFor(YAML), managedBy: ownedBy });
    await user.type(await screen.findByLabelText('yaml'), 'x');
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await user.click(screen.getByRole('button', { name: 'Apply anyway' }));

    expect(screen.getByText('Applying your changes to Deployment web.')).toBeInTheDocument();
  });

  it('asks for the object name instead of a single click', async () => {
    const user = userEvent.setup();
    renderYaml();

    await user.click(screen.getByRole('button', { name: 'Delete' }));

    expect(screen.getByText('Deleting Deployment web.')).toBeInTheDocument();
    expect(screen.queryByText('Delete web?')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  });

  it('deletes with the typed confirmation once the name matches', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(okResponse({}));
    vi.stubGlobal('fetch', fetchMock);
    const { onDeleted } = renderYaml();

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.type(screen.getByLabelText('Name'), 'web');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(onDeleted).toHaveBeenCalled();
    const url = String(fetchMock.mock.calls.at(-1)?.[0]);
    expect(url).toContain('confirm=web');
  });

  it('drops the question when it is cancelled', async () => {
    const user = userEvent.setup();
    renderYaml();

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Delete' })).toBeInTheDocument();
  });

  it('asks for the object name before applying, as it does before deleting', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(screen.getByText('Applying your changes to Deployment web.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Confirm' })).toBeDisabled();
  });

  it('applies with the typed confirmation once the name matches', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(okResponse({}));
    vi.stubGlobal('fetch', fetchMock);
    const { onApplied } = renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await user.type(screen.getByLabelText('Name'), 'web');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(onApplied).toHaveBeenCalled();
    const url = String(fetchMock.mock.calls.at(-1)?.[0]);
    expect(url).toContain('confirm=web');
  });

  it('drops the apply question when it is cancelled', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn().mockResolvedValue(okResponse({}));
    vi.stubGlobal('fetch', fetchMock);
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    const applied = fetchMock.mock.calls.some((call) => {
      const options = call[1] as RequestInit | undefined;
      return options?.method === 'PUT';
    });
    expect(applied).toBe(false);
  });
});

describe('editing and deleting what the cluster would refuse', () => {
  const deploymentKey = accessKey('p-mk1', target);

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

  it('greys out Delete and says why', () => {
    useAccessStore.getState().setRefused(deploymentKey, {
      delete: 'requires container.deployments.delete in Cloud IAM',
    });
    renderYaml();

    const button = screen.getByRole('button', { name: 'Delete' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', 'requires container.deployments.delete in Cloud IAM');
  });

  it('greys out Apply even after an edit', async () => {
    const user = userEvent.setup();
    useAccessStore.getState().setRefused(deploymentKey, { edit: 'no updating here' });
    renderYaml();

    await user.type(screen.getByRole('textbox'), 'x');

    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('leaves Delete alone when nothing stands in the way', () => {
    useAccessStore.getState().setRefused(deploymentKey, { edit: 'no updating here' });
    renderYaml();

    expect(screen.getByRole('button', { name: 'Delete' })).toBeEnabled();
  });
});

describe('applying to an object a gitops controller owns', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  function applied(): boolean {
    return fetchMock.mock.calls.some((call) => {
      const options = call[1] as RequestInit | undefined;
      return options?.method === 'PUT';
    });
  }

  beforeEach(() => {
    fetchMock = vi.fn().mockResolvedValue(okResponse(detailFor(YAML)));
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('says what will put the change back before applying', async () => {
    const user = userEvent.setup();
    renderYaml(YAML, { ...detailFor(YAML), managedBy: ownedBy });
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    expect(
      screen.getByText('Application argocd/podinfo will put this back at the next reconcile.'),
    ).toBeInTheDocument();
    expect(applied()).toBe(false);
  });

  it('applies once the warning is acknowledged', async () => {
    const user = userEvent.setup();
    renderYaml(YAML, { ...detailFor(YAML), managedBy: ownedBy });
    await user.type(await screen.findByLabelText('yaml'), 'x');
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await user.click(screen.getByRole('button', { name: 'Apply anyway' }));

    await waitFor(() => {
      expect(applied()).toBe(true);
    });
  });

  it('drops the change when the warning is cancelled', async () => {
    const user = userEvent.setup();
    renderYaml(YAML, { ...detailFor(YAML), managedBy: ownedBy });
    await user.type(await screen.findByLabelText('yaml'), 'x');
    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(
      screen.queryByText('Application argocd/podinfo will put this back at the next reconcile.'),
    ).not.toBeInTheDocument();
    expect(applied()).toBe(false);
  });

  it('leaves an unowned object straight through', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(await screen.findByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Apply' }));

    await waitFor(() => {
      expect(applied()).toBe(true);
    });
  });
});

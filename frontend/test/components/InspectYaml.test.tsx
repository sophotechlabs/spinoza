import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

interface EditorStubProps {
  value: string;
  onChange: (value: string | undefined) => void;
  options: { readOnly: boolean };
}

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

function renderYaml(yaml = YAML) {
  const onApplied = vi.fn();
  const onDeleted = vi.fn();
  const view = render(
    <InspectYaml
      target={target}
      detail={detailFor(yaml)}
      onApplied={onApplied}
      onDeleted={onDeleted}
    />,
  );
  return { onApplied, onDeleted, view };
}

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

  it('starts clean with the actions disabled', () => {
    renderYaml();

    expect(screen.getByLabelText('yaml')).toHaveValue(YAML);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Revert' })).toBeDisabled();
    expect(screen.queryByText('unsaved changes')).not.toBeInTheDocument();
  });

  it('enables the actions once the draft diverges', async () => {
    const user = userEvent.setup();
    renderYaml();

    await user.type(screen.getByLabelText('yaml'), 'x');

    expect(screen.getByRole('button', { name: 'Apply' })).toBeEnabled();
    expect(screen.getByText('unsaved changes')).toBeInTheDocument();
  });

  it('reverts the draft', async () => {
    const user = userEvent.setup();
    renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');

    await user.click(screen.getByRole('button', { name: 'Revert' }));

    expect(screen.getByLabelText('yaml')).toHaveValue(YAML);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });

  it('applies the draft and reports success', async () => {
    const user = userEvent.setup();
    const { onApplied } = renderYaml();
    await user.type(screen.getByLabelText('yaml'), 'x');

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

  it('deletes on confirmation', async () => {
    const user = userEvent.setup();
    const { onDeleted } = renderYaml();
    await user.click(screen.getByRole('button', { name: 'Delete' }));

    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(onDeleted).toHaveBeenCalledTimes(1);
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

  it('reseeds the draft when the object is refetched', async () => {
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

    expect(screen.getByLabelText('yaml')).toHaveValue(next);
    expect(screen.getByRole('button', { name: 'Apply' })).toBeDisabled();
  });
});

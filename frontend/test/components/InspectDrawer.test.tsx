import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
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

import InspectDrawer from '../../src/components/InspectDrawer';
import type { ObjectDetail, ObjectRef } from '../../src/lib/types';

const target: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'flux-system',
  name: 'web',
};

const detail: ObjectDetail = {
  apiVersion: 'v1',
  kind: 'Pod',
  name: 'web',
  namespace: 'flux-system',
  uid: 'pod-uid',
  createdAt: '2026-07-27T09:00:00Z',
  labels: { app: 'web' },
  yaml: 'kind: Pod\n',
};

function stubApi(objectPayload: unknown = detail, eventsPayload: unknown = []): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/events')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve(eventsPayload) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(objectPayload) });
    }),
  );
}

function renderDrawer(ref: ObjectRef | null = target) {
  const onClose = vi.fn();
  const onDeleted = vi.fn();
  const view = render(<InspectDrawer target={ref} onClose={onClose} onDeleted={onDeleted} />);
  return { onClose, onDeleted, view };
}

describe('InspectDrawer', () => {
  beforeEach(() => {
    stubApi();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('prompts for a selection when nothing is targeted', () => {
    renderDrawer(null);

    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('shows a loading state then the overview', async () => {
    renderDrawer();

    expect(screen.getByText('Loading web…')).toBeInTheDocument();
    expect(await screen.findByText('Metadata')).toBeInTheDocument();
    expect(screen.getByText('app')).toBeInTheDocument();
  });

  it('surfaces a fetch failure', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'pods "web" not found' }),
      }),
    );
    renderDrawer();

    expect(await screen.findByText('pods "web" not found')).toBeInTheDocument();
  });

  it('falls back to a generic message for a non-Error rejection', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue('nope'));
    renderDrawer();

    expect(await screen.findByText('object request failed')).toBeInTheDocument();
  });

  it('switches to the yaml tab', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'YAML' }));

    expect(screen.getByLabelText('yaml')).toHaveValue('kind: Pod\n');
  });

  it('switches to the events tab', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Events' }));

    expect(await screen.findByText('No events for this object.')).toBeInTheDocument();
  });

  it('refetches after an apply', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'YAML' }));
    const calls = (globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.length;

    await user.type(screen.getByLabelText('yaml'), 'x');
    await user.click(screen.getByRole('button', { name: 'Apply' }));
    await screen.findByText('Applied.');

    expect((globalThis.fetch as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(
      calls + 1,
    );
  });

  it('reports the delete up to the caller', async () => {
    const user = userEvent.setup();
    const { onDeleted } = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'YAML' }));

    await user.click(screen.getByRole('button', { name: 'Delete' }));
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    expect(onDeleted).toHaveBeenCalledTimes(1);
  });

  it('closes on request', async () => {
    const user = userEvent.setup();
    const { onClose } = renderDrawer();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Close' }));

    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('clears the detail when the target is removed', async () => {
    const { view } = renderDrawer();
    await screen.findByText('Metadata');

    view.rerender(<InspectDrawer target={null} onClose={vi.fn()} onDeleted={vi.fn()} />);

    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('resizes with the keyboard', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');
    const drawer = screen.getByRole('complementary');
    const handle = screen.getByRole('button', { name: 'Resize inspector' });
    const initial = drawer.style.width;

    handle.focus();
    await user.keyboard('{ArrowLeft}');
    const widened = drawer.style.width;
    expect(widened).not.toBe(initial);

    await user.keyboard('{ArrowRight}');
    expect(drawer.style.width).toBe(initial);

    await user.keyboard('{ArrowUp}');
    expect(drawer.style.width).toBe(initial);
  });

  it('resizes on a handle drag', async () => {
    renderDrawer();
    await screen.findByText('Metadata');
    const drawer = screen.getByRole('complementary');
    const initial = drawer.style.width;

    fireEvent.mouseDown(screen.getByRole('button', { name: 'Resize inspector' }), {
      clientX: 900,
    });
    fireEvent.mouseMove(window, { clientX: 800 });

    expect(drawer.style.width).not.toBe(initial);
  });
  it('offers port forwarding for a pod with ports', async () => {
    stubApi({ ...detail, ports: [{ name: 'http', port: 8080, protocol: 'TCP' }] });
    renderDrawer();

    expect(await screen.findByText('Ports')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Forward' })).toBeInTheDocument();
    expect(screen.getByText('8080 · http')).toBeInTheDocument();
  });

  it('hides port forwarding for a kind that cannot be forwarded', async () => {
    stubApi({
      ...detail,
      apiVersion: 'apps/v1',
      kind: 'Deployment',
      ports: [{ port: 8080 }],
    });
    renderDrawer();

    await screen.findByText('Metadata');
    expect(screen.queryByText('Ports')).not.toBeInTheDocument();
  });

  it('hides port forwarding when the object has no ports', async () => {
    stubApi({ ...detail, ports: [] });
    renderDrawer();

    await screen.findByText('Metadata');
    expect(screen.queryByText('Ports')).not.toBeInTheDocument();
  });
});

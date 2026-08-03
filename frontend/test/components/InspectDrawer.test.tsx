import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';
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
import { INSPECT_LOGS_SUB_ID } from '../../src/components/InspectLogs';
import { TAIL_LINES } from '../../src/lib/useLogStream';
import { useLogsStore } from '../../src/store/logs';
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
  containers: ['app', 'sidecar'],
  yaml: 'kind: Pod\n',
};

function stubApi(objectPayload: unknown = detail, eventsPayload: unknown = []): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/metrics/history')) {
        return Promise.resolve({
          ok: true,
          json: () =>
            Promise.resolve({ namespace: 'flux-system', pod: 'web', cpu: [], memory: [] }),
        });
      }
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
  const subscribeLogs = vi.fn();
  const unsubscribeLogs = vi.fn();
  const view = render(
    <InspectDrawer
      target={ref}
      subscribeLogs={subscribeLogs}
      unsubscribeLogs={unsubscribeLogs}
      onClose={onClose}
      onDeleted={onDeleted}
    />,
  );
  return { onClose, onDeleted, subscribeLogs, unsubscribeLogs, view };
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

  it('switches to the metrics tab', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Metrics' }));

    expect(await screen.findByLabelText('Metric range')).toBeInTheDocument();
  });

  it('drops the old object before the new one arrives', async () => {
    const user = userEvent.setup();
    const view = renderDrawer();
    await screen.findByText('Metadata');

    view.view.rerender(
      <InspectDrawer target={{ ...target, name: 'api' }} onClose={vi.fn()} onDeleted={vi.fn()} />,
    );

    expect(screen.getByText(/Loading api/)).toBeInTheDocument();
    expect(screen.queryByText('Metadata')).not.toBeInTheDocument();
    await user.click(await screen.findByRole('button', { name: 'YAML' }));
  });

  it('never hands the yaml editor one object with another one selected', async () => {
    const user = userEvent.setup();
    const view = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'YAML' }));
    expect(screen.getByLabelText('yaml')).toHaveValue('kind: Pod\n');

    view.view.rerender(
      <InspectDrawer target={{ ...target, name: 'api' }} onClose={vi.fn()} onDeleted={vi.fn()} />,
    );

    expect(screen.queryByLabelText('yaml')).not.toBeInTheDocument();
  });

  it('cancels a pending delete when a different object is selected', async () => {
    const user = userEvent.setup();
    const view = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'YAML' }));
    await user.click(screen.getByRole('button', { name: 'Delete' }));
    expect(screen.getByText('Delete web?')).toBeInTheDocument();

    view.view.rerender(
      <InspectDrawer target={{ ...target, name: 'api' }} onClose={vi.fn()} onDeleted={vi.fn()} />,
    );

    expect(screen.queryByText(/Delete/)).not.toBeInTheDocument();
  });

  it('drops the action panel when a different object is selected', async () => {
    stubApi({ ...detail, kind: 'Deployment', apiVersion: 'apps/v1' });
    const view = render(
      <InspectDrawer
        target={{ ...target, group: 'apps', resource: 'deployments' }}
        onClose={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );
    expect(await screen.findByLabelText('replicas')).toBeInTheDocument();

    view.rerender(
      <InspectDrawer
        target={{ ...target, group: 'apps', resource: 'deployments', name: 'other' }}
        onClose={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );

    expect(screen.queryByLabelText('replicas')).not.toBeInTheDocument();
  });

  it('falls back to Overview when the new kind has no such tab', async () => {
    const user = userEvent.setup();
    const view = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Metrics' }));
    expect(await screen.findByLabelText('Metric range')).toBeInTheDocument();

    stubApi({ ...detail, kind: 'Deployment', name: 'web' });
    view.view.rerender(
      <InspectDrawer
        target={{ ...target, resource: 'deployments', name: 'web-2' }}
        onClose={vi.fn()}
        onDeleted={vi.fn()}
      />,
    );

    await screen.findByText('Metadata');
    expect(screen.queryByLabelText('Metric range')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Metrics' })).not.toBeInTheDocument();
  });

  it('collapses to a strip and comes back', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');

    await user.click(screen.getByLabelText('Hide inspector'));

    expect(screen.queryByText('Metadata')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Show inspector')).toBeInTheDocument();

    await user.click(screen.getByLabelText('Show inspector'));

    expect(await screen.findByText('Metadata')).toBeInTheDocument();
  });

  it('collapses from the empty state too', async () => {
    const user = userEvent.setup();
    renderDrawer(null);
    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();

    await user.click(screen.getByLabelText('Hide inspector'));

    expect(screen.queryByText('Select a row to inspect it.')).not.toBeInTheDocument();
    expect(screen.getByLabelText('Show inspector')).toBeInTheDocument();
  });

  it('stays collapsed when a different object is selected', async () => {
    const user = userEvent.setup();
    const view = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByLabelText('Hide inspector'));

    view.view.rerender(
      <InspectDrawer target={{ ...target, name: 'api' }} onClose={vi.fn()} onDeleted={vi.fn()} />,
    );

    expect(screen.getByLabelText('Show inspector')).toBeInTheDocument();
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

describe('InspectDrawer logs tab', () => {
  beforeEach(() => {
    stubApi();
    useLogsStore.setState({ streams: new Map() });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('streams the pod log once the tab is opened', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDrawer();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Logs' }));

    expect(subscribeLogs).toHaveBeenCalledWith(INSPECT_LOGS_SUB_ID, {
      namespace: 'flux-system',
      name: 'web',
      container: 'app',
      tailLines: TAIL_LINES,
      follow: true,
    });
  });

  it('renders streamed lines and stops the stream when the tab is left', async () => {
    const user = userEvent.setup();
    const { unsubscribeLogs } = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().appendLines(INSPECT_LOGS_SUB_ID, ['hello from the pod']);
    });
    expect(screen.getByText('hello from the pod')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Overview' }));

    expect(unsubscribeLogs).toHaveBeenCalledWith(INSPECT_LOGS_SUB_ID);
  });

  it('pauses following without dropping the stream', async () => {
    const user = userEvent.setup();
    const { subscribeLogs, unsubscribeLogs } = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Logs' }));
    subscribeLogs.mockClear();
    unsubscribeLogs.mockClear();

    await user.click(screen.getByRole('button', { name: 'Following' }));

    expect(screen.getByRole('button', { name: 'Follow' })).toBeInTheDocument();
    expect(subscribeLogs).not.toHaveBeenCalled();
    expect(unsubscribeLogs).not.toHaveBeenCalled();
  });

  it('switches container and resubscribes', async () => {
    const user = userEvent.setup();
    const { subscribeLogs } = renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    await user.selectOptions(screen.getByLabelText('Log container'), 'sidecar');

    expect(subscribeLogs).toHaveBeenLastCalledWith(
      INSPECT_LOGS_SUB_ID,
      expect.objectContaining({ container: 'sidecar' }),
    );
  });

  it('surfaces a stream failure', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().failStream(INSPECT_LOGS_SUB_ID, 'pods/log is forbidden');
    });

    expect(screen.getByText('pods/log is forbidden')).toBeInTheDocument();
  });

  it('marks an ended stream', async () => {
    const user = userEvent.setup();
    renderDrawer();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Logs' }));

    act(() => {
      useLogsStore.getState().startStream(INSPECT_LOGS_SUB_ID);
      useLogsStore.getState().endStream(INSPECT_LOGS_SUB_ID);
    });

    expect(screen.getByText('stream ended')).toBeInTheDocument();
  });

  it('prefers the live container list over the fetched detail', async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const onDeleted = vi.fn();
    const subscribeLogs = vi.fn();
    render(
      <InspectDrawer
        target={target}
        containers={[
          { name: 'app', state: 'running', ready: true, restarts: 0, init: false },
          {
            name: 'debugger',
            state: 'running',
            ready: true,
            restarts: 0,
            init: false,
            ephemeral: true,
          },
        ]}
        subscribeLogs={subscribeLogs}
        unsubscribeLogs={vi.fn()}
        onClose={onClose}
        onDeleted={onDeleted}
      />,
    );
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Logs' }));
    await user.selectOptions(screen.getByLabelText('Log container'), 'debugger');

    expect(subscribeLogs).toHaveBeenLastCalledWith(
      INSPECT_LOGS_SUB_ID,
      expect.objectContaining({ container: 'debugger' }),
    );
  });

  it('offers no logs tab for a non-pod', async () => {
    stubApi({ ...detail, kind: 'Deployment', containers: undefined });
    renderDrawer();
    await screen.findByText('Metadata');

    expect(screen.queryByRole('button', { name: 'Logs' })).not.toBeInTheDocument();
  });
});

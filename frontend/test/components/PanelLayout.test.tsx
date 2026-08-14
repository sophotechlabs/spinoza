import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('../../src/components/ForwardsPanel', () => ({
  default: ({ active }: { active: boolean }) => (
    <div data-testid="forwards-panel">{active ? 'polling' : 'idle'}</div>
  ),
}));

vi.mock('../../src/components/TerminalPanel', () => ({
  default: ({ target }: { target: { pod: string; container: string } }) => (
    <div data-testid="terminal-panel">
      {target.pod}/{target.container}
    </div>
  ),
}));

vi.mock('../../src/components/InspectYaml', () => ({
  default: () => <div data-testid="yaml-panel" />,
}));

vi.mock('../../src/components/InspectMetrics', () => ({
  default: () => <div data-testid="metrics-panel" />,
}));

import PanelLayout from '../../src/components/PanelLayout';
import { PLACEMENT_KEY } from '../../src/lib/panels';
import { usePanelsStore } from '../../src/store/panels';
import { useToastsStore } from '../../src/store/toasts';
import type { ObjectRef } from '../../src/lib/types';
import { makeRow, parentOf } from '../helpers';

const podRef: ObjectRef = {
  group: '',
  version: 'v1',
  resource: 'pods',
  namespace: 'prod',
  name: 'web',
};

const detail = {
  apiVersion: 'v1',
  kind: 'Pod',
  name: 'web',
  namespace: 'prod',
  uid: 'uid-web',
  createdAt: '2026-08-03T09:00:00Z',
  containers: ['app'],
  yaml: 'kind: Pod\n',
};

function stubApi(): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (url.startsWith('/api/events')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
      }
      if (url.startsWith('/api/exec/support')) {
        return Promise.resolve({ ok: true, json: () => Promise.resolve({ shell: 'present' }) });
      }
      return Promise.resolve({ ok: true, json: () => Promise.resolve(detail) });
    }),
  );
}

function renderLayout(overrides: Partial<Parameters<typeof PanelLayout>[0]> = {}) {
  const onClose = vi.fn();
  const onDeleted = vi.fn();
  const view = render(
    <PanelLayout
      selection={{
        ref: podRef,
        row: makeRow({
          uid: 'uid-web',
          name: 'web',
          namespace: 'prod',
          containers: [
            { name: 'app', state: 'running', ready: true, restarts: 0, init: false },
            { name: 'sidecar', state: 'running', ready: true, restarts: 0, init: false },
          ],
        }),
      }}
      subscribeLogs={vi.fn()}
      unsubscribeLogs={vi.fn()}
      onClose={onClose}
      onDeleted={onDeleted}
      {...overrides}
    >
      <div data-testid="main-area" />
    </PanelLayout>,
  );
  return { onClose, onDeleted, view };
}

function dockStrip(side: 'left' | 'right' | 'bottom'): HTMLElement {
  return screen.getByRole('tablist', { name: `${side} dock` });
}

const PANEL_TYPE = 'application/x-spinoza-panel';

function transfer(id: string) {
  return {
    types: [PANEL_TYPE],
    getData: () => id,
    setData: vi.fn(),
    effectAllowed: 'none',
  };
}

describe('PanelLayout', () => {
  beforeEach(() => {
    window.localStorage.clear();
    usePanelsStore.getState().reset();
    window.localStorage.clear();
    useToastsStore.getState().clear();
    stubApi();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('docks the object panels right and the pod panels at the bottom', async () => {
    renderLayout();
    await screen.findByText('Metadata');

    const right = dockStrip('right');
    expect(within(right).getByRole('tab', { name: 'Overview' })).toBeInTheDocument();
    expect(within(right).getByRole('tab', { name: 'Metrics' })).toBeInTheDocument();

    const bottom = dockStrip('bottom');
    expect(within(bottom).getByRole('tab', { name: 'Terminal' })).toBeInTheDocument();
  });

  it('starts with the left dock empty and offered as a drop target', () => {
    renderLayout();

    expect(screen.getByLabelText('Empty left dock')).toBeInTheDocument();
  });

  it('moves a panel to another dock from the strip control', async () => {
    const user = userEvent.setup();
    renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Move Overview to the left' }));

    const left = dockStrip('left');
    expect(within(left).queryByRole('tab', { name: 'YAML' })).toBeNull();
    expect(screen.queryByLabelText('Empty left dock')).toBeNull();
  });

  it('moves a panel by dragging its tab onto another dock', async () => {
    renderLayout();
    await screen.findByText('Metadata');
    const empty = screen.getByLabelText('Empty left dock');

    fireEvent.dragOver(empty, { dataTransfer: transfer('metrics') });
    fireEvent.drop(empty, { dataTransfer: transfer('metrics') });

    const left = dockStrip('left');
    expect(within(left).queryByRole('tab', { name: 'YAML' })).toBeNull();
  });

  it('remembers a move for the next session', async () => {
    const user = userEvent.setup();
    renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Move Overview to the bottom' }));

    expect(window.localStorage.getItem(PLACEMENT_KEY)).toContain('"overview":"bottom"');
  });

  it('opens the panel it was asked to move', async () => {
    const user = userEvent.setup();
    renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Move Overview to the bottom' }));

    const bottom = dockStrip('bottom');
    expect(
      within(bottom).getByRole('button', { name: 'Move Overview to the left' }),
    ).toBeInTheDocument();
  });

  it('keeps a panel alive across a move', async () => {
    const user = userEvent.setup();
    renderLayout();
    await user.click(screen.getByRole('tab', { name: 'Terminal' }));
    const before = await screen.findByTestId('terminal-panel');

    await user.click(screen.getByRole('button', { name: 'Move Terminal to the left' }));

    expect(screen.getByTestId('terminal-panel')).toBe(before);
  });

  it('greys out the pod panels for something that is not a pod', async () => {
    stubApi();
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ ...detail, kind: 'Deployment', containers: undefined }),
      }),
    );
    renderLayout({ selection: { ref: podRef, row: null } });
    await screen.findByText('Metadata');

    expect(screen.getByRole('tab', { name: 'Logs' })).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('tab', { name: 'Metrics' })).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute('aria-disabled', 'true');
  });

  it('greys out every object panel with nothing selected', () => {
    renderLayout({ selection: null });

    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getByRole('tab', { name: 'YAML' })).toHaveAttribute('aria-disabled', 'true');
    expect(screen.getAllByText('Select a row to inspect it.').length).toBeGreaterThan(0);
  });

  it('surfaces a failed object fetch', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 404,
        json: () => Promise.resolve({ message: 'pods "web" not found' }),
      }),
    );
    renderLayout();

    expect(await screen.findByText('pods "web" not found')).toBeInTheDocument();
  });

  it('polls forwards only while its panel is open', async () => {
    const user = userEvent.setup();
    renderLayout();

    expect(screen.getByTestId('forwards-panel')).toHaveTextContent('polling');

    await user.click(screen.getByRole('tab', { name: 'Terminal' }));

    expect(screen.getByTestId('forwards-panel')).toHaveTextContent('idle');
  });

  it('opens the events panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('tab', { name: 'Events' }));

    expect(await screen.findByText('No events for this object.')).toBeInTheDocument();
  });

  it('opens the yaml panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('tab', { name: 'YAML' }));

    expect(screen.getByTestId('yaml-panel')).toBeInTheDocument();
  });

  it('opens the metrics panel', async () => {
    const user = userEvent.setup();
    renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('tab', { name: 'Metrics' }));

    expect(screen.getByTestId('metrics-panel')).toBeInTheDocument();
  });

  it('offers a forward for a pod that declares ports', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string) => {
        if (url.startsWith('/api/portforward')) {
          return Promise.resolve({ ok: true, json: () => Promise.resolve([]) });
        }
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({ ...detail, ports: [{ name: 'http', port: 8080 }] }),
        });
      }),
    );
    renderLayout();
    await screen.findByText('Metadata');

    expect(screen.getByRole('button', { name: 'Forward' })).toBeInTheDocument();
  });

  it('offers no forward for a pod with an empty port list', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ ...detail, ports: [] }),
      }),
    );
    renderLayout();
    await screen.findByText('Metadata');

    expect(screen.queryByRole('button', { name: 'Forward' })).toBeNull();
  });

  it('renders the view it wraps', () => {
    renderLayout();

    expect(screen.getByTestId('main-area')).toBeInTheDocument();
  });

  it('closes the selection from the panel header', async () => {
    const user = userEvent.setup();
    const { onClose } = renderLayout();
    await screen.findByText('Metadata');

    await user.click(screen.getByRole('button', { name: 'Close' }));

    expect(onClose).toHaveBeenCalled();
  });
});

describe('the layout a session leaves behind', () => {
  beforeEach(() => {
    window.localStorage.clear();
    usePanelsStore.getState().reset();
    window.localStorage.clear();
    useToastsStore.getState().clear();
    stubApi();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('reopens on the tab that was left open', async () => {
    const user = userEvent.setup();
    const first = renderLayout();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('tab', { name: 'Events' }));
    await screen.findByText('No events for this object.');
    first.view.unmount();

    renderLayout();

    expect(await screen.findByText('No events for this object.')).toBeInTheDocument();
  });

  it('reopens with the dock still collapsed', async () => {
    const user = userEvent.setup();
    const first = renderLayout();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Hide the right dock' }));
    first.view.unmount();

    renderLayout();

    expect(await screen.findByRole('button', { name: 'Show the right dock' })).toBeInTheDocument();
  });

  it('reopens at the dock size the last drag left', async () => {
    const first = renderLayout();
    await screen.findByText('Metadata');
    const handle = screen.getByRole('button', { name: 'Resize the right dock' });
    fireEvent.mouseDown(handle, { clientX: 900 });
    fireEvent.mouseMove(window, { clientX: 800, buttons: 1 });
    fireEvent.mouseUp(window);
    first.view.unmount();

    renderLayout();

    const frame = parentOf(screen.getByRole('button', { name: 'Resize the right dock' }));
    expect(frame.style.width).toBe('660px');
  });

  it('forgets all of it when the layout is reset', async () => {
    const user = userEvent.setup();
    const first = renderLayout();
    await screen.findByText('Metadata');
    await user.click(screen.getByRole('button', { name: 'Hide the right dock' }));
    first.view.unmount();

    act(() => {
      usePanelsStore.getState().reset();
    });
    renderLayout();

    expect(await screen.findByRole('button', { name: 'Hide the right dock' })).toBeInTheDocument();
  });
});

describe('PanelLayout width', () => {
  it('lets the middle column shrink instead of overflowing the page', () => {
    const { view } = renderLayout();

    const root = view.container.firstElementChild as HTMLElement;
    expect(root.className).toContain('min-w-0');
    expect(root.className).toContain('overflow-hidden');
  });
});

describe('an object deleted out from under the panels', () => {
  beforeEach(() => {
    stubApi();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('replaces the overview with a plain statement instead of a ghost object', async () => {
    const { view } = renderLayout();
    await screen.findByText('Metadata');

    view.rerender(
      <PanelLayout
        selection={{ ref: podRef, row: null }}
        subscribeLogs={vi.fn()}
        unsubscribeLogs={vi.fn()}
        onClose={vi.fn()}
        onDeleted={vi.fn()}
      >
        <div data-testid="main-area" />
      </PanelLayout>,
    );

    expect(screen.getByText('This object is no longer in the cluster.')).toBeInTheDocument();
    expect(screen.queryByText('Metadata')).not.toBeInTheDocument();
  });
});

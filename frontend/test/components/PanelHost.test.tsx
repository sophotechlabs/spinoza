import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import PanelHost from '../../src/components/PanelHost';
import type { PanelTab } from '../../src/components/PanelHost';
import type { DockSide, PanelId } from '../../src/lib/panels';

function tabs(...ids: PanelId[]): PanelTab[] {
  return ids.map((id) => ({ id, disabled: false, title: id }));
}

function renderHost(overrides: Partial<Parameters<typeof PanelHost>[0]> = {}) {
  const onActivate = vi.fn();
  const onMove = vi.fn();
  const hostRef = vi.fn();
  const view = render(
    <PanelHost
      side="right"
      tabs={tabs('overview', 'yaml')}
      active="overview"
      onActivate={onActivate}
      onMove={onMove}
      hostRef={hostRef}
      emptyHint="Select a row to inspect it."
      {...overrides}
    />,
  );
  return { onActivate, onMove, hostRef, view };
}

function dataTransfer(payload: Record<string, string>) {
  return {
    types: Object.keys(payload),
    getData: (type: string) => payload[type] ?? '',
    setData: vi.fn(),
    effectAllowed: 'none',
  };
}

const PANEL_TYPE = 'application/x-spinoza-panel';

function parentOf(node: HTMLElement): HTMLElement {
  const parent = node.parentElement;
  if (parent === null) {
    throw new Error('expected the element to have a parent');
  }
  return parent;
}

describe('PanelHost', () => {
  it('renders one tab per docked panel', () => {
    renderHost();

    expect(screen.getByRole('tab', { name: 'Overview' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'YAML' })).toBeInTheDocument();
  });

  it('reports the tab that was clicked', async () => {
    const user = userEvent.setup();
    const { onActivate } = renderHost();

    await user.click(screen.getByRole('tab', { name: 'YAML' }));

    expect(onActivate).toHaveBeenCalledWith('yaml');
  });

  it('refuses a disabled tab but keeps it hoverable so its reason can be read', async () => {
    const user = userEvent.setup();
    const { onActivate } = renderHost({
      tabs: [{ id: 'logs', disabled: true, title: 'Select a pod to see this' }],
      active: null,
    });
    const tab = screen.getByRole('tab', { name: 'Logs' });
    expect(tab).toHaveAttribute('aria-disabled', 'true');
    expect(tab).not.toBeDisabled();
    expect(tab).toHaveAttribute('title', 'Select a pod to see this');

    await user.click(screen.getByRole('tab', { name: 'Logs' }));

    expect(onActivate).not.toHaveBeenCalled();
  });

  it('shows the hint while nothing is open', () => {
    renderHost({ active: null });

    expect(screen.getByText('Select a row to inspect it.')).toBeInTheDocument();
  });

  it('hides the hint once a panel is open', () => {
    renderHost();

    expect(screen.queryByText('Select a row to inspect it.')).not.toBeInTheDocument();
  });

  it('offers the two other docks for the open panel', () => {
    renderHost();

    expect(screen.getByRole('button', { name: 'Move Overview to the left' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Move Overview to the bottom' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Move Overview to the right' })).toBeNull();
  });

  it('moves the open panel from the strip control', async () => {
    const user = userEvent.setup();
    const { onMove } = renderHost();

    await user.click(screen.getByRole('button', { name: 'Move Overview to the bottom' }));

    expect(onMove).toHaveBeenCalledWith('overview', 'bottom');
  });

  it('carries the panel id when a tab is dragged', () => {
    renderHost();
    const transfer = dataTransfer({});

    fireEvent.dragStart(screen.getByRole('tab', { name: 'YAML' }), {
      dataTransfer: transfer,
    });

    expect(transfer.setData).toHaveBeenCalledWith(PANEL_TYPE, 'yaml');
  });

  it('accepts a panel dropped on the tab strip', () => {
    const { onMove } = renderHost({ side: 'bottom' });
    const strip = parentOf(screen.getByRole('tab', { name: 'Overview' }));

    fireEvent.dragOver(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'metrics' }) });
    fireEvent.drop(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'metrics' }) });

    expect(onMove).toHaveBeenCalledWith('metrics', 'bottom');
  });

  it('ignores a drag that carries something else', () => {
    const { onMove } = renderHost();
    const strip = parentOf(screen.getByRole('tab', { name: 'Overview' }));

    fireEvent.dragOver(strip, { dataTransfer: dataTransfer({ 'text/plain': 'hello' }) });
    fireEvent.drop(strip, { dataTransfer: dataTransfer({ 'text/plain': 'hello' }) });

    expect(onMove).not.toHaveBeenCalled();
  });

  it('clears the drop highlight when the drag leaves', () => {
    renderHost();
    const strip = parentOf(screen.getByRole('tab', { name: 'Overview' }));

    fireEvent.dragOver(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'metrics' }) });
    expect(strip.className).toContain('bg-neutral-800');

    fireEvent.dragLeave(strip);

    expect(strip.className).not.toContain('bg-neutral-800');
  });

  it('collapses and reopens', async () => {
    const user = userEvent.setup();
    renderHost();

    await user.click(screen.getByRole('button', { name: 'Hide the right dock' }));
    expect(screen.queryByRole('tab', { name: 'Overview' })).toBeNull();

    await user.click(screen.getByRole('button', { name: 'Show the right dock' }));

    expect(screen.getByRole('tab', { name: 'Overview' })).toBeInTheDocument();
  });

  it('takes a drop while collapsed', async () => {
    const user = userEvent.setup();
    const { onMove } = renderHost();
    await user.click(screen.getByRole('button', { name: 'Hide the right dock' }));
    const strip = parentOf(screen.getByRole('button', { name: 'Show the right dock' }));

    fireEvent.dragOver(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'terminal' }) });
    fireEvent.drop(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'terminal' }) });

    expect(onMove).toHaveBeenCalledWith('terminal', 'right');
  });

  it('offers an empty dock as a drop target', () => {
    const { onMove } = renderHost({ side: 'left', tabs: [], active: null });
    const strip = screen.getByLabelText('Empty left dock');

    fireEvent.dragOver(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'events' }) });
    fireEvent.drop(strip, { dataTransfer: dataTransfer({ [PANEL_TYPE]: 'events' }) });

    expect(onMove).toHaveBeenCalledWith('events', 'left');
  });

  it('ignores a drop that carries no panel', () => {
    const { onMove } = renderHost({ side: 'left', tabs: [], active: null });
    const strip = screen.getByLabelText('Empty left dock');

    fireEvent.drop(strip, { dataTransfer: dataTransfer({}) });

    expect(onMove).not.toHaveBeenCalled();
  });

  it('resizes a side dock horizontally', () => {
    renderHost();
    const handle = screen.getByRole('button', { name: 'Resize the right dock' });

    fireEvent.mouseDown(handle, { clientX: 900 });
    fireEvent.mouseMove(window, { clientX: 800 });

    const frame = parentOf(handle);
    expect(frame.style.width).toBe('660px');
  });

  it('resizes the bottom dock vertically', () => {
    renderHost({ side: 'bottom' });
    const handle = screen.getByRole('button', { name: 'Resize the bottom dock' });

    fireEvent.mouseDown(handle, { clientY: 600 });
    fireEvent.mouseMove(window, { clientY: 500 });

    const frame = parentOf(handle);
    expect(frame.style.height).toBe('340px');
  });

  it('nudges a side dock with the arrow keys', () => {
    renderHost();
    const handle = screen.getByRole('button', { name: 'Resize the right dock' });
    const frame = parentOf(handle);

    fireEvent.keyDown(handle, { key: 'ArrowRight' });
    expect(frame.style.width).toBe('592px');

    fireEvent.keyDown(handle, { key: 'ArrowLeft' });
    expect(frame.style.width).toBe('560px');
  });

  it('nudges the bottom dock with the arrow keys', () => {
    renderHost({ side: 'bottom' });
    const handle = screen.getByRole('button', { name: 'Resize the bottom dock' });
    const frame = parentOf(handle);

    fireEvent.keyDown(handle, { key: 'ArrowUp' });
    expect(frame.style.height).toBe('272px');

    fireEvent.keyDown(handle, { key: 'ArrowDown' });
    expect(frame.style.height).toBe('240px');
  });

  it('ignores an unrelated key on the resize handle', () => {
    renderHost();
    const handle = screen.getByRole('button', { name: 'Resize the right dock' });
    const frame = parentOf(handle);

    fireEvent.keyDown(handle, { key: 'Enter' });

    expect(frame.style.width).toBe('560px');
  });

  it('puts the left dock handle after its column', () => {
    const sides: DockSide[] = ['left', 'right'];
    for (const side of sides) {
      const { view } = renderHost({ side });
      const handle = screen.getByRole('button', { name: `Resize the ${side} dock` });
      const frame = parentOf(handle);
      const index = [...frame.children].indexOf(handle);
      if (side === 'left') {
        expect(index).toBe(1);
      } else {
        expect(index).toBe(0);
      }
      view.unmount();
    }
  });
});

describe('PanelHost chrome per side', () => {
  it('dresses an empty dock for each side', () => {
    const sides: DockSide[] = ['left', 'right', 'bottom'];
    const edges = ['border-r', 'border-l', 'border-t'];
    sides.forEach((side, index) => {
      const { view } = renderHost({ side, tabs: [], active: null });
      const strip = screen.getByLabelText(`Empty ${side} dock`);
      expect(strip.className).toContain(edges[index]);
      view.unmount();
    });
  });

  it('dresses a collapsed dock for each side', async () => {
    const user = userEvent.setup();
    const sides: DockSide[] = ['left', 'right', 'bottom'];
    const glyphs = ['›', '‹', '▸'];
    for (const [index, side] of sides.entries()) {
      const { view } = renderHost({ side });
      await user.click(screen.getByRole('button', { name: `Hide the ${side} dock` }));
      const button = screen.getByRole('button', { name: `Show the ${side} dock` });
      expect(button).toHaveTextContent(glyphs[index]);
      view.unmount();
    }
  });

  it('points the collapse control the way the dock closes', async () => {
    const user = userEvent.setup();
    const { view } = renderHost({ side: 'left' });
    expect(screen.getByRole('button', { name: 'Hide the left dock' })).toHaveTextContent('‹');
    view.unmount();

    renderHost({ side: 'bottom' });
    expect(screen.getByRole('button', { name: 'Hide the bottom dock' })).toHaveTextContent('▾');
    await user.click(screen.getByRole('button', { name: 'Hide the bottom dock' }));
    expect(screen.getByRole('button', { name: 'Show the bottom dock' })).toBeInTheDocument();
  });
});

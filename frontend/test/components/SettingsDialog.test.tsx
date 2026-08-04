import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SettingsDialog from '../../src/components/SettingsDialog';
import { useThemeStore } from '../../src/store/theme';
import { useSettingsStore } from '../../src/store/settings';
import { usePanelsStore } from '../../src/store/panels';
import { DEFAULT_PLACEMENT } from '../../src/lib/panels';

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});
const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

beforeEach(() => {
  window.localStorage.clear();
  showModal.mockClear();
  close.mockClear();
  HTMLDialogElement.prototype.showModal = showModal;
  HTMLDialogElement.prototype.close = close;
});

afterEach(() => {
  act(() => {
    useThemeStore.getState().setPreference('dark');
    useSettingsStore.getState().setLogView('pretty');
    usePanelsStore.getState().reset();
  });
  window.localStorage.clear();
});

function open() {
  const onClose = vi.fn();
  render(<SettingsDialog open onClose={onClose} />);
  return { onClose };
}

describe('the settings dialog', () => {
  it('opens on Appearance and offers the other sections', () => {
    open();

    expect(screen.getByRole('button', { name: 'Appearance' })).toHaveAttribute(
      'aria-current',
      'true',
    );
    expect(screen.getByLabelText('Theme preference')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Logs' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Panels' })).toBeInTheDocument();
  });

  it('changes the theme from Appearance', async () => {
    const user = userEvent.setup();
    open();

    await user.selectOptions(screen.getByLabelText('Theme preference'), 'light');

    expect(useThemeStore.getState().preference).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('remembers a default log view', async () => {
    const user = userEvent.setup();
    open();

    await user.click(screen.getByRole('button', { name: 'Logs' }));
    await user.selectOptions(screen.getByLabelText('Default log view'), 'raw');

    expect(useSettingsStore.getState().logView).toBe('raw');
  });

  it('puts the docks back where they started', async () => {
    const user = userEvent.setup();
    act(() => {
      usePanelsStore.getState().move('logs', 'left');
    });
    open();

    await user.click(screen.getByRole('button', { name: 'Panels' }));
    await user.click(screen.getByRole('button', { name: 'Reset' }));

    expect(usePanelsStore.getState().placement).toEqual(DEFAULT_PLACEMENT);
  });

  it('reports being dismissed', async () => {
    const user = userEvent.setup();
    const { onClose } = open();

    await user.click(screen.getByRole('button', { name: 'Close' }));

    expect(onClose).toHaveBeenCalled();
  });

  it('stays shut until it is asked to open', () => {
    render(<SettingsDialog open={false} onClose={vi.fn()} />);

    expect(showModal).not.toHaveBeenCalled();
  });
});

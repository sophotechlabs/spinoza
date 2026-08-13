import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
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
    expect(screen.getByLabelText('Theme preference')).toHaveValue('dark');
    expect(screen.getByRole('option', { name: 'Light' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'System' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Logs' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Panels' })).toBeInTheDocument();
  });

  it('lists the themes alphabetically, with System last', () => {
    open();

    const select = screen.getByLabelText('Theme preference');
    const names = Array.from(select.querySelectorAll('option')).map((option) => option.textContent);
    expect(names).toEqual([
      'Blade Runner',
      'Borg',
      'Cyberpunk',
      'Dark',
      'Light',
      'Matrix',
      'Nord',
      'Skywalker',
      'Startrekker',
      'System',
    ]);
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

describe('importing a theme', () => {
  const SOLARIZED = JSON.stringify({
    id: 'solarized',
    name: 'Solarized Light',
    base: 'light',
    tokens: { surface: '#fdf6e3' },
  });

  afterEach(() => {
    act(() => {
      for (const theme of useThemeStore.getState().custom) {
        useThemeStore.getState().removeTheme(theme.id);
      }
      useThemeStore.getState().setPreference('dark');
    });
  });

  it('installs it, selects it and lists it', async () => {
    const user = userEvent.setup();
    open();

    await user.click(screen.getByLabelText('Import a theme'));
    await user.paste(SOLARIZED);
    await user.click(screen.getByRole('button', { name: 'Import' }));

    expect(useThemeStore.getState().custom).toHaveLength(1);
    expect(useThemeStore.getState().preference).toBe('solarized');
    expect(document.documentElement.style.getPropertyValue('--surface')).toBe('#fdf6e3');
    expect(screen.getByRole('option', { name: 'Solarized Light' })).toBeInTheDocument();
  });

  it('says why it refused a broken file', async () => {
    const user = userEvent.setup();
    open();

    await user.click(screen.getByLabelText('Import a theme'));
    await user.paste('{"id":"x","name":"X","base":"sideways"}');
    await user.click(screen.getByRole('button', { name: 'Import' }));

    expect(screen.getByText('base must be "dark" or "light"')).toBeInTheDocument();
    expect(useThemeStore.getState().custom).toHaveLength(0);
  });

  it('says so when the file is not JSON at all', async () => {
    const user = userEvent.setup();
    open();

    await user.click(screen.getByLabelText('Import a theme'));
    await user.paste('nonsense');
    await user.click(screen.getByRole('button', { name: 'Import' }));

    expect(screen.getByText('that is not valid JSON')).toBeInTheDocument();
  });

  it('warns when a theme leaves the background to its base', async () => {
    const user = userEvent.setup();
    open();

    await user.click(screen.getByLabelText('Import a theme'));
    await user.paste('{"id":"accent","name":"Accent","base":"dark","tokens":{"ok":"#00ff00"}}');
    await user.click(screen.getByRole('button', { name: 'Import' }));

    expect(screen.getByText(/inherits the background of its base/)).toBeInTheDocument();
  });

  it('removes one on request', async () => {
    const user = userEvent.setup();
    open();
    await user.click(screen.getByLabelText('Import a theme'));
    await user.paste(SOLARIZED);
    await user.click(screen.getByRole('button', { name: 'Import' }));

    await user.click(screen.getByRole('button', { name: 'Remove Solarized Light' }));

    expect(useThemeStore.getState().custom).toHaveLength(0);
  });

  it('installs one picked from a file', async () => {
    const user = userEvent.setup();
    open();
    const file = new File([SOLARIZED], 'solarized.json', { type: 'application/json' });

    await user.upload(screen.getByLabelText('Theme file'), file);

    expect(await screen.findByRole('option', { name: 'Solarized Light' })).toBeInTheDocument();
    expect(useThemeStore.getState().preference).toBe('solarized');
    expect(document.documentElement.style.getPropertyValue('--surface')).toBe('#fdf6e3');
  });

  it('leaves a rejected file in the box so it can be fixed', async () => {
    const user = userEvent.setup();
    open();
    const file = new File(['nonsense'], 'broken.json', { type: 'application/json' });

    await user.upload(screen.getByLabelText('Theme file'), file);

    expect(await screen.findByText('that is not valid JSON')).toBeInTheDocument();
    expect(screen.getByLabelText('Import a theme')).toHaveValue('nonsense');
  });

  it('does nothing when the file dialog is dismissed', () => {
    open();

    fireEvent.change(screen.getByLabelText('Theme file'), { target: { files: [] } });

    expect(useThemeStore.getState().custom).toHaveLength(0);
    expect(screen.getByLabelText('Import a theme')).toHaveValue('');
  });

  it('reaches the file dialog from the button', async () => {
    const user = userEvent.setup();
    const opened = vi.fn();
    open();
    screen.getByLabelText('Theme file').addEventListener('click', opened);

    await user.click(screen.getByRole('button', { name: 'Choose file…' }));

    expect(opened).toHaveBeenCalled();
  });

  it('copies the current theme out as json', async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    open();

    await user.click(screen.getByRole('button', { name: 'Copy current as JSON' }));

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('"id": "dark"'));
  });
});

describe('the keyboard section', () => {
  it('opens straight there when the app asks for it', () => {
    render(<SettingsDialog open section="Keyboard" onClose={vi.fn()} />);

    expect(screen.getByText('Open the command palette')).toBeInTheDocument();
    expect(screen.getByText('Ctrl K')).toBeInTheDocument();
  });

  it('follows the app when it asks for a different section', () => {
    const view = render(<SettingsDialog open section="Appearance" onClose={vi.fn()} />);
    expect(screen.getByLabelText('Theme preference')).toBeInTheDocument();

    view.rerender(<SettingsDialog open section="Keyboard" onClose={vi.fn()} />);

    expect(screen.getByText('Open the command palette')).toBeInTheDocument();
  });
});

describe('the about section', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('names the interface version and the backend it is talking to', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve({ version: 'v1.4.0' }) }),
    );
    render(<SettingsDialog open section="About" onClose={vi.fn()} />);

    expect(screen.getByText('test')).toBeInTheDocument();
    expect(await screen.findByText('v1.4.0')).toBeInTheDocument();
  });

  it('leaves the backend row blank when the endpoint is not there', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: false, status: 404, json: () => Promise.resolve({}) }),
    );
    render(<SettingsDialog open section="About" onClose={vi.fn()} />);

    expect(screen.getByText('test')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByText('—')).toBeInTheDocument();
    });
  });

  it('does not ask for the version while it is closed', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    render(<SettingsDialog open={false} section="About" onClose={vi.fn()} />);

    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('the terminal section', () => {
  afterEach(() => {
    act(() => {
      useSettingsStore.getState().setScreenReader(false);
    });
  });

  it('offers screen reader mode, off to begin with', () => {
    render(<SettingsDialog open section="Terminal" onClose={vi.fn()} />);

    expect(screen.getByLabelText('Screen reader mode')).not.toBeChecked();
  });

  it('remembers the choice', async () => {
    const user = userEvent.setup();
    render(<SettingsDialog open section="Terminal" onClose={vi.fn()} />);

    await user.click(screen.getByLabelText('Screen reader mode'));

    expect(useSettingsStore.getState().screenReader).toBe(true);
    expect(screen.getByLabelText('Screen reader mode')).toBeChecked();
  });
});

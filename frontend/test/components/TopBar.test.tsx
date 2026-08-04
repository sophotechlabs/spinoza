import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import TopBar from '../../src/components/TopBar';
import { useThemeStore } from '../../src/store/theme';
import { emitSystemDark } from '../helpers';

vi.mock('../../src/components/ContextPicker', () => ({
  default: ({ onSwitched }: { onSwitched: () => void }) => (
    <button type="button" onClick={onSwitched}>
      switch context
    </button>
  ),
}));

function dotFor(container: HTMLElement): Element {
  const dot = container.querySelector('[data-testid="connection-dot"]');
  if (!dot) {
    throw new Error('status dot not found');
  }
  return dot;
}

describe('TopBar', () => {
  it('shows a green dot when connected', () => {
    const { container } = render(<TopBar status="connected" />);
    expect(screen.getByText('connected')).toBeInTheDocument();
    expect(dotFor(container).className).toContain('bg-ok-solid');
  });

  it('shows a yellow dot when connecting', () => {
    const { container } = render(<TopBar status="connecting" />);
    expect(screen.getByText('connecting')).toBeInTheDocument();
    expect(dotFor(container).className).toContain('bg-warn-solid');
  });

  it('shows a red dot when disconnected', () => {
    const { container } = render(<TopBar status="disconnected" />);
    expect(screen.getByText('disconnected')).toBeInTheDocument();
    expect(dotFor(container).className).toContain('bg-error-solid');
  });

  it('shows the active view when one is provided', () => {
    render(<TopBar status="connected" view="gitops" />);
    expect(screen.getByText('gitops')).toBeInTheDocument();
  });

  it('calls onReconnect when the reconnect button is clicked', async () => {
    const user = userEvent.setup();
    const onReconnect = vi.fn();
    render(<TopBar status="disconnected" onReconnect={onReconnect} />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(onReconnect).toHaveBeenCalledTimes(1);
  });

  it('does not throw when reconnect is clicked without a handler', async () => {
    const user = userEvent.setup();
    render(<TopBar status="disconnected" />);
    await user.click(screen.getByRole('button', { name: 'Reconnect' }));
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
  });
});

describe('TopBar context switch', () => {
  it('reports the switch to the owner', async () => {
    const user = userEvent.setup();
    const onContextChanged = vi.fn();
    const onReconnect = vi.fn();
    render(
      <TopBar status="connected" onReconnect={onReconnect} onContextChanged={onContextChanged} />,
    );

    await user.click(screen.getByRole('button', { name: 'switch context' }));

    expect(onContextChanged).toHaveBeenCalledOnce();
    expect(onReconnect).not.toHaveBeenCalled();
  });

  it('falls back to a reconnect when no owner is listening', async () => {
    const user = userEvent.setup();
    const onReconnect = vi.fn();
    render(<TopBar status="connected" onReconnect={onReconnect} />);

    await user.click(screen.getByRole('button', { name: 'switch context' }));

    expect(onReconnect).toHaveBeenCalledOnce();
  });
});

describe('the theme picker', () => {
  afterEach(() => {
    act(() => {
      useThemeStore.getState().setPreference('dark');
    });
  });

  it('offers every theme and starts on the stored one', () => {
    render(<TopBar status="connected" />);
    const picker = screen.getByLabelText('Theme');

    expect(picker).toHaveValue('dark');
    expect(screen.getByRole('option', { name: 'light' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'system' })).toBeInTheDocument();
  });

  it('repaints the document when a theme is chosen', async () => {
    const user = userEvent.setup();
    render(<TopBar status="connected" />);

    await user.selectOptions(screen.getByLabelText('Theme'), 'light');

    expect(useThemeStore.getState().preference).toBe('light');
    expect(document.documentElement.dataset.theme).toBe('light');
  });

  it('hands the choice to the operating system when asked', async () => {
    const user = userEvent.setup();
    render(<TopBar status="connected" />);
    act(() => {
      emitSystemDark(true);
    });

    await user.selectOptions(screen.getByLabelText('Theme'), 'system');

    expect(useThemeStore.getState().resolved).toBe('dark');
    act(() => {
      emitSystemDark(false);
    });
  });
});

describe('the settings dialog from the top bar', () => {
  const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
    this.open = true;
  });
  const close = vi.fn(function close(this: HTMLDialogElement) {
    this.open = false;
  });

  beforeEach(() => {
    showModal.mockClear();
    close.mockClear();
    HTMLDialogElement.prototype.showModal = showModal;
    HTMLDialogElement.prototype.close = close;
  });

  it('opens from the gear and closes again', async () => {
    const user = userEvent.setup();
    render(<TopBar status="connected" />);

    await user.click(screen.getByRole('button', { name: 'Settings' }));
    expect(showModal).toHaveBeenCalled();

    await user.click(screen.getByRole('button', { name: 'Close' }));
    expect(close).toHaveBeenCalled();
  });
});

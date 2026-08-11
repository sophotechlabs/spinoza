import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Toasts, { TOAST_TTL_MS } from '../../src/components/Toasts';
import {
  MAX_TOASTS,
  notifyError,
  notifyOk,
  notifyWarn,
  useToastsStore,
} from '../../src/store/toasts';
import { parentOf } from '../helpers';

describe('Toasts', () => {
  beforeEach(() => {
    useToastsStore.getState().clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    useToastsStore.getState().clear();
  });

  it('shows nothing until something is reported', () => {
    render(<Toasts />);

    expect(screen.getByLabelText('Notifications')).toBeEmptyDOMElement();
  });

  it('announces a success and a failure', () => {
    render(<Toasts />);

    act(() => {
      notifyOk('pod-a deleted');
      notifyError('cannot delete pod-a');
    });

    expect(screen.getByText('pod-a deleted')).toBeInTheDocument();
    expect(screen.getByText('cannot delete pod-a')).toBeInTheDocument();
    expect(screen.getByLabelText('Notifications')).toHaveAttribute('aria-live', 'polite');
  });

  it('colours a failure differently from a success', () => {
    render(<Toasts />);

    act(() => {
      notifyOk('scaled to 3');
    });
    const ok = parentOf(screen.getByText('scaled to 3'));

    act(() => {
      notifyError('scale failed');
    });
    const failed = parentOf(screen.getByText('scale failed'));

    expect(ok.className).toContain('border-ok-line');
    expect(failed.className).toContain('border-error-line');
  });

  it('colours a warning apart from both', () => {
    render(<Toasts />);

    act(() => {
      notifyWarn('attached to an existing debug container');
    });
    const warned = parentOf(screen.getByText('attached to an existing debug container'));

    expect(warned.className).toContain('border-warn-line');
    expect(warned.className).not.toContain('border-error-line');
  });

  it('dismisses one from its close button', async () => {
    const user = userEvent.setup();
    render(<Toasts />);
    act(() => {
      notifyOk('forward started');
    });

    await user.click(screen.getByRole('button', { name: 'Dismiss: forward started' }));

    expect(screen.queryByText('forward started')).not.toBeInTheDocument();
  });

  it('lets a toast expire on its own', () => {
    vi.useFakeTimers();
    render(<Toasts />);
    act(() => {
      notifyOk('restart requested');
    });

    act(() => {
      vi.advanceTimersByTime(TOAST_TTL_MS);
    });

    expect(screen.queryByText('restart requested')).not.toBeInTheDocument();
  });

  it('keeps only the newest few so the corner cannot fill up', () => {
    render(<Toasts />);

    act(() => {
      let index = 0;
      while (index < MAX_TOASTS + 2) {
        notifyOk(`event ${String(index)}`);
        index += 1;
      }
    });

    expect(screen.getAllByRole('button')).toHaveLength(MAX_TOASTS);
    expect(screen.queryByText('event 0')).not.toBeInTheDocument();
    expect(screen.getByText(`event ${String(MAX_TOASTS + 1)}`)).toBeInTheDocument();
  });
});

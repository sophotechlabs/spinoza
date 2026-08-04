import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import CopyButton from '../../src/components/CopyButton';
import { useToastsStore } from '../../src/store/toasts';

const writeText = vi.fn().mockResolvedValue(undefined);

beforeEach(() => {
  writeText.mockClear();
  Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
  useToastsStore.getState().clear();
});

afterEach(() => {
  useToastsStore.getState().clear();
});

describe('CopyButton', () => {
  it('names what it copies for a screen reader', () => {
    render(<CopyButton what="the UID" text="uid-web" />);

    expect(screen.getByRole('button', { name: 'Copy the UID' })).toBeInTheDocument();
  });

  it('copies on click and reports it', async () => {
    const user = userEvent.setup();
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    render(<CopyButton what="the UID" text="uid-web" />);

    await user.click(screen.getByRole('button', { name: 'Copy the UID' }));

    expect(writeText).toHaveBeenCalledWith('uid-web');
    await waitFor(() => {
      expect(useToastsStore.getState().toasts).toHaveLength(1);
    });
  });

  it('hides until hovered when it sits inside busy content', () => {
    render(<CopyButton what="the name" text="web" quiet />);

    expect(screen.getByRole('button', { name: 'Copy the name' }).className).toContain('opacity-0');
  });

  it('stays visible in a toolbar', () => {
    render(<CopyButton what="YAML" text="kind: Pod" />);

    expect(screen.getByRole('button', { name: 'Copy YAML' }).className).not.toContain('opacity-0');
  });
});

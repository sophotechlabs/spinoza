import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import SecretData from '../../src/components/SecretData';
import type { SecretEntry } from '../../src/lib/types';
import { useToastsStore } from '../../src/store/toasts';

const MASK = '••••••••••••';

function entries(): SecretEntry[] {
  return [
    { key: 'password', value: 'hunter2', bytes: 7 },
    { key: 'keystore', value: '/v8A', bytes: 3, binary: true },
  ];
}

function renderData(uid = 'uid-1') {
  return render(<SecretData uid={uid} entries={entries()} />);
}

beforeEach(() => {
  useToastsStore.getState().clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  useToastsStore.getState().clear();
});

describe('a secret in the overview', () => {
  it('keeps every value masked until it is asked for', () => {
    renderData();

    expect(screen.getByLabelText('password')).toHaveValue(MASK);
    expect(screen.getByLabelText('keystore')).toHaveValue(MASK);
    expect(screen.queryByDisplayValue('hunter2')).not.toBeInTheDocument();
  });

  it('shows one value without showing the others', async () => {
    const user = userEvent.setup();
    renderData();

    await user.click(screen.getByRole('button', { name: 'Show password' }));

    expect(screen.getByLabelText('password')).toHaveValue('hunter2');
    expect(screen.getByLabelText('keystore')).toHaveValue(MASK);
  });

  it('hides it again', async () => {
    const user = userEvent.setup();
    renderData();
    await user.click(screen.getByRole('button', { name: 'Show password' }));

    await user.click(screen.getByRole('button', { name: 'Hide password' }));

    expect(screen.getByLabelText('password')).toHaveValue(MASK);
  });

  it('leaves the value readable but not editable', () => {
    renderData();

    expect(screen.getByLabelText('password')).toHaveAttribute('readonly');
  });

  it('says how big each value is, and which one is not text', () => {
    renderData();

    expect(screen.getByText('7 bytes')).toBeInTheDocument();
    expect(screen.getByText('3 bytes, shown as base64')).toBeInTheDocument();
  });

  it('counts a single byte in the singular', () => {
    render(<SecretData uid="uid-1" entries={[{ key: 'a', value: 'x', bytes: 1 }]} />);

    expect(screen.getByText('1 byte')).toBeInTheDocument();
  });

  it('offers a copy for each value', () => {
    renderData();

    expect(screen.getByRole('button', { name: 'Copy password' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy keystore' })).toBeInTheDocument();
  });

  it('copies the value itself, not the mask', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });
    renderData();

    await user.click(screen.getByRole('button', { name: 'Copy password' }));

    expect(writeText).toHaveBeenCalledWith('hunter2');
  });

  it('masks everything again when another secret is opened', async () => {
    const user = userEvent.setup();
    const view = renderData();
    await user.click(screen.getByRole('button', { name: 'Show password' }));

    view.rerender(<SecretData uid="uid-2" entries={entries()} />);

    expect(screen.getByLabelText('password')).toHaveValue(MASK);
  });
});

describe('a value with more than one line', () => {
  const yamlValue = 'apiVersion: v1\nkind: Config\nspec:\n  auth: true\n';

  it('stays on one masked line until it is shown', () => {
    render(
      <SecretData uid="uid-1" entries={[{ key: 'config.yaml', value: yamlValue, bytes: 44 }]} />,
    );

    expect(screen.getByLabelText('config.yaml').tagName).toBe('INPUT');
  });

  it('opens into a box that keeps the line breaks', async () => {
    const user = userEvent.setup();
    render(
      <SecretData uid="uid-1" entries={[{ key: 'config.yaml', value: yamlValue, bytes: 44 }]} />,
    );

    await user.click(screen.getByRole('button', { name: 'Show config.yaml' }));

    const field = screen.getByLabelText('config.yaml');
    expect(field.tagName).toBe('TEXTAREA');
    expect(field).toHaveValue(yamlValue);
    expect(field).toHaveAttribute('rows', '5');
  });

  it('stops growing at twelve rows', async () => {
    const user = userEvent.setup();
    const long = Array.from({ length: 40 }, (_, index) => `line ${String(index)}`).join('\n');
    render(<SecretData uid="uid-1" entries={[{ key: 'long', value: long, bytes: long.length }]} />);

    await user.click(screen.getByRole('button', { name: 'Show long' }));

    expect(screen.getByLabelText('long')).toHaveAttribute('rows', '12');
  });
});

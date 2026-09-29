import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ReadableNamespaces from '../../src/components/ReadableNamespaces';
import { readNamespaces, writeNamespaces } from '../../src/lib/settings';
import { resetStored, startSaving, stopSaving } from '../../src/lib/persist';
import { useNamespaceStore } from '../../src/store/namespace';

function stubSettings(): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(() =>
    Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ values: {} }) }),
  );
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  resetStored();
  stubSettings();
});

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    stopSaving();
    resetStored();
    useNamespaceStore.getState().reset();
  });
});

describe('ReadableNamespaces', () => {
  it('shows what was saved for this context', async () => {
    await writeNamespaces({ 'spinoza-eks-editor': ['payments', 'storefront'], other: ['shop'] });

    render(<ReadableNamespaces context="spinoza-eks-editor" />);

    expect(screen.getByLabelText('Namespaces you can read')).toHaveValue('payments, storefront');
  });

  it('names the context it is for', () => {
    render(<ReadableNamespaces context="spinoza-eks-editor" />);

    expect(
      screen.getByText(/For when spinoza-eks-editor cannot list namespaces/),
    ).toBeInTheDocument();
  });

  it('saves the names for this context only and asks for namespaces again', async () => {
    const user = userEvent.setup();
    await writeNamespaces({ other: ['shop'] });
    render(<ReadableNamespaces context="spinoza-eks-editor" />);
    const before = useNamespaceStore.getState().asked;

    await user.type(
      screen.getByLabelText('Namespaces you can read'),
      'payments,  storefront payments',
    );
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Saved.');
    });
    expect(readNamespaces()).toEqual({
      other: ['shop'],
      'spinoza-eks-editor': ['payments', 'storefront'],
    });
    expect(screen.getByLabelText('Namespaces you can read')).toHaveValue('payments, storefront');
    expect(useNamespaceStore.getState().asked).toBe(before + 1);
  });

  it('sends the names to the server', async () => {
    const user = userEvent.setup();
    const fetchMock = stubSettings();
    startSaving();
    render(<ReadableNamespaces context="spinoza-eks-editor" />);

    await user.type(screen.getByLabelText('Namespaces you can read'), 'payments');
    await user.keyboard('{Enter}');

    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent('Saved.');
    });
    const sent = fetchMock.mock.calls.map((call) => {
      const body = (call[1] as RequestInit).body;
      if (typeof body !== 'string') {
        return '';
      }
      return body;
    });
    expect(sent.some((body) => body.includes('spinoza.namespaces.v1'))).toBe(true);
    expect(sent.some((body) => body.includes('payments'))).toBe(true);
  });

  it('refuses a name that is not a namespace and saves nothing', async () => {
    const user = userEvent.setup();
    render(<ReadableNamespaces context="spinoza-eks-editor" />);
    const before = useNamespaceStore.getState().asked;

    await user.type(screen.getByLabelText('Namespaces you can read'), 'payments Payments');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    expect(screen.getByRole('alert')).toHaveTextContent('Payments is not a namespace name.');
    expect(readNamespaces()).toEqual({});
    expect(useNamespaceStore.getState().asked).toBe(before);
  });

  it('names every wrong name at once', async () => {
    const user = userEvent.setup();
    render(<ReadableNamespaces context="spinoza-eks-editor" />);

    await user.type(screen.getByLabelText('Namespaces you can read'), 'A b_c');
    await user.click(screen.getByRole('button', { name: 'Save' }));

    expect(screen.getByRole('alert')).toHaveTextContent('A, b_c are not namespace names.');
  });

  it('clears the list for this context when the field is emptied', async () => {
    const user = userEvent.setup();
    await writeNamespaces({ 'spinoza-eks-editor': ['payments'] });
    render(<ReadableNamespaces context="spinoza-eks-editor" />);

    await user.clear(screen.getByLabelText('Namespaces you can read'));
    await user.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(readNamespaces()).toEqual({});
    });
  });

  it('stops saying saved once the field changes again', async () => {
    const user = userEvent.setup();
    render(<ReadableNamespaces context="spinoza-eks-editor" />);
    await user.type(screen.getByLabelText('Namespaces you can read'), 'payments');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => {
      expect(screen.getByRole('status')).toBeInTheDocument();
    });

    await user.type(screen.getByLabelText('Namespaces you can read'), ' web');

    expect(screen.queryByRole('status')).toBeNull();
  });

  it('is closed until a context is open', () => {
    render(<ReadableNamespaces context="" />);

    expect(screen.getByLabelText('Namespaces you can read')).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled();
    expect(screen.getByText(/For an account that cannot list namespaces/)).toBeInTheDocument();
  });
});

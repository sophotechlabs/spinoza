import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ColumnSettings from '../../src/components/ColumnSettings';
import { clearCatalog, rememberCatalog } from '../../src/store/catalog';
import { readColumns, writeColumns } from '../../src/lib/settings';
import { resetStored } from '../../src/lib/persist';
import { makeCategory, makeDescriptor } from '../helpers';

function seedCatalog(): void {
  rememberCatalog([
    makeCategory('Workloads', [
      makeDescriptor({ group: '', version: 'v1', resource: 'pods', kind: 'Pod' }),
      makeDescriptor({
        group: 'apps',
        version: 'v1',
        resource: 'deployments',
        kind: 'Deployment',
      }),
    ]),
  ]);
}

function stubSettings(): ReturnType<typeof vi.fn> {
  const fetchMock = vi.fn(() =>
    Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ values: {} }) }),
  );
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  resetStored();
  seedCatalog();
  stubSettings();
});

afterEach(() => {
  vi.unstubAllGlobals();
  clearCatalog();
  resetStored();
});

describe('ColumnSettings', () => {
  it('offers every discovered kind to add a column to', () => {
    render(<ColumnSettings />);

    const picker = screen.getByLabelText('Kind');
    expect(picker).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Pod' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Deployment · apps' })).toBeInTheDocument();
  });

  it('says the kind shows what it comes with until a column is added', () => {
    render(<ColumnSettings />);

    expect(screen.getByText(/shows what Deployment comes with/)).toBeInTheDocument();
  });

  it('keeps a column that was added', async () => {
    const user = userEvent.setup();
    render(<ColumnSettings />);
    await user.selectOptions(screen.getByLabelText('Kind'), '/v1/pods');

    await user.type(screen.getByLabelText('Column name'), 'App');
    await user.type(screen.getByLabelText('Field path'), '.metadata.labels.app');
    await user.click(screen.getByRole('button', { name: 'Add' }));

    expect(screen.getByText('App')).toBeInTheDocument();
    expect(screen.getByText('.metadata.labels.app')).toBeInTheDocument();
    await waitFor(() => {
      expect(readColumns()['/v1/pods']).toEqual([{ name: 'App', path: '.metadata.labels.app' }]);
    });
  });

  it('will not add a column with no name or no path', async () => {
    const user = userEvent.setup();
    render(<ColumnSettings />);

    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled();

    await user.type(screen.getByLabelText('Column name'), 'App');
    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled();
  });

  it('removes a column again', async () => {
    const user = userEvent.setup();
    await writeColumns({ '/v1/pods': [{ name: 'App', path: '.metadata.labels.app' }] });
    render(<ColumnSettings />);
    await user.selectOptions(screen.getByLabelText('Kind'), '/v1/pods');
    expect(screen.getByText('App')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Remove App' }));

    expect(screen.queryByText('App')).not.toBeInTheDocument();
  });

  it('stops adding once the kind has as many as it will take', async () => {
    const user = userEvent.setup();
    const many = Array.from({ length: 8 }, (_, at) => ({
      name: `c${String(at)}`,
      path: '.spec.nodeName',
    }));
    await writeColumns({ '/v1/pods': many });
    render(<ColumnSettings />);
    await user.selectOptions(screen.getByLabelText('Kind'), '/v1/pods');

    await user.type(screen.getByLabelText('Column name'), 'One more');
    await user.type(screen.getByLabelText('Field path'), '.spec.nodeName');

    expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled();
  });

  it('says so when nothing has been discovered yet', () => {
    clearCatalog();

    render(<ColumnSettings />);

    expect(screen.getByText('No kinds discovered yet.')).toBeInTheDocument();
  });
});

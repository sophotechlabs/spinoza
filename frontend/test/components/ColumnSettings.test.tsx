import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ColumnSettings from '../../src/components/ColumnSettings';
import { clearCatalog, rememberCatalog } from '../../src/store/catalog';
import { readColumns, writeColumns } from '../../src/lib/settings';
import { resetStored } from '../../src/lib/persist';
import { makeCategory, makeDescriptor, rejectsWith } from '../helpers';
import { useClustersStore } from '../../src/store/clusters';
import { bumpClusterEpoch, useClusterStore } from '../../src/store/cluster';

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
  useClustersStore.setState({ active: '' });
  useClusterStore.getState().reset();
  resetStored();
  seedCatalog();
  stubSettings();
});

afterEach(() => {
  vi.unstubAllGlobals();
  act(() => {
    clearCatalog();
    useClustersStore.setState({ active: '' });
    useClusterStore.getState().reset();
    resetStored();
  });
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

  it('reads the cluster itself rather than waiting for another view to do it', async () => {
    clearCatalog();
    const fetchMock = vi.fn(() =>
      Promise.resolve({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            categories: [
              {
                name: 'Workloads',
                resources: [
                  {
                    group: '',
                    version: 'v1',
                    resource: 'pods',
                    kind: 'Pod',
                    namespaced: true,
                    category: 'Workloads',
                  },
                ],
              },
            ],
          }),
      }),
    );
    vi.stubGlobal('fetch', fetchMock);

    render(<ColumnSettings />);

    expect(await screen.findByRole('option', { name: 'Pod' })).toBeInTheDocument();
  });

  it('says why when the cluster cannot be read', async () => {
    clearCatalog();
    const fetchMock = vi
      .fn()
      .mockResolvedValue({ ok: false, status: 503, json: () => Promise.resolve({}) });
    vi.stubGlobal('fetch', fetchMock);

    render(<ColumnSettings />);

    expect(await screen.findByText(/status 503/)).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('says something even when the failure is not an error', async () => {
    clearCatalog();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => rejectsWith('not an Error')()),
    );

    render(<ColumnSettings />);

    expect(await screen.findByText('the discovery request failed')).toBeInTheDocument();
  });

  it('waits rather than claiming there is nothing', () => {
    clearCatalog();
    vi.stubGlobal(
      'fetch',
      vi.fn(() => new Promise(() => undefined)),
    );

    render(<ColumnSettings />);

    expect(screen.getByText('Reading what this cluster has…')).toBeInTheDocument();
  });

  it('does not store an old discovery answer under the replacement cluster', async () => {
    clearCatalog();
    useClustersStore.setState({ active: 'first' });
    let finishFirst: (response: unknown) => void = () => undefined;
    const first = new Promise((resolve) => {
      finishFirst = resolve;
    });
    const second = {
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          categories: [
            {
              name: 'Network',
              resources: [
                {
                  group: '',
                  version: 'v1',
                  resource: 'services',
                  kind: 'Service',
                  namespaced: true,
                  category: 'Network',
                },
              ],
            },
          ],
        }),
    };
    vi.stubGlobal(
      'fetch',
      vi
        .fn()
        .mockImplementationOnce(() => first)
        .mockResolvedValue(second),
    );
    render(<ColumnSettings />);

    act(() => {
      useClustersStore.setState({ active: 'second' });
      bumpClusterEpoch();
    });

    expect(await screen.findByRole('option', { name: 'Service' })).toBeInTheDocument();
    await act(async () => {
      finishFirst({
        ok: true,
        status: 200,
        json: () =>
          Promise.resolve({
            categories: [
              {
                name: 'Workloads',
                resources: [
                  {
                    group: '',
                    version: 'v1',
                    resource: 'pods',
                    kind: 'Pod',
                    namespaced: true,
                    category: 'Workloads',
                  },
                ],
              },
            ],
          }),
      });
      await first;
    });

    expect(screen.queryByRole('option', { name: 'Pod' })).not.toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'Service' })).toBeInTheDocument();
  });

  it('drops a discovery failure that lands after unmount', async () => {
    clearCatalog();
    let fail: (reason: unknown) => void = () => undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn(
        () =>
          new Promise((_resolve, reject) => {
            fail = reject;
          }),
      ),
    );
    const view = render(<ColumnSettings />);

    view.unmount();
    await act(async () => {
      fail(new Error('too late'));
      await Promise.resolve();
    });

    expect(screen.queryByText('too late')).not.toBeInTheDocument();
  });
});

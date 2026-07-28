import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { FluxDashboard } from '../../src/lib/types';
import FluxOverview from '../../src/components/FluxOverview';
import { makeFluxResource } from '../helpers';

function stubFlux(dashboard: FluxDashboard): void {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({ ok: true, json: () => Promise.resolve(dashboard) }),
  );
}

const dashboard: FluxDashboard = {
  groups: [
    {
      name: 'Sources',
      ready: 1,
      total: 2,
      resources: [
        makeFluxResource({
          kind: 'GitRepository',
          name: 'repo-a',
          ready: 'True',
          revision: 'main@sha1:abc',
          source: '',
          createdAt: '2026-07-24T09:00:00Z',
        }),
        makeFluxResource({
          kind: 'HelmRelease',
          name: 'rel-b',
          ready: 'False',
          revision: '1.2.3',
          source: 'HelmRepository/x',
          createdAt: '2026-07-20T09:00:00Z',
        }),
      ],
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('FluxOverview', () => {
  it('shows a loading state then the resource tiles', async () => {
    stubFlux(dashboard);
    render(<FluxOverview onSelect={vi.fn()} />);
    expect(screen.getByText('Loading Flux resources…')).toBeInTheDocument();
    expect(await screen.findByText('repo-a')).toBeInTheDocument();
    expect(screen.getByText('rel-b')).toBeInTheDocument();
    expect(screen.getByText('Sources')).toBeInTheDocument();
    expect(screen.getByText('1/2 ready')).toBeInTheDocument();
    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.getByText('Not ready')).toBeInTheDocument();
    expect(screen.getByText('2026-07-24')).toBeInTheDocument();
  });

  it('shows the error message when the fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('flux down')));
    render(<FluxOverview onSelect={vi.fn()} />);
    expect(await screen.findByText('flux down')).toBeInTheDocument();
  });

  it('shows an empty message when there are no groups', async () => {
    stubFlux({ groups: [] });
    render(<FluxOverview onSelect={vi.fn()} />);
    expect(await screen.findByText('No Flux resources found.')).toBeInTheDocument();
  });
  it('reports the clicked tile to the caller', async () => {
    stubFlux(dashboard);
    const onSelect = vi.fn();
    render(<FluxOverview onSelect={onSelect} />);

    await userEvent.click(await screen.findByRole('button', { name: /repo-a/ }));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ name: 'repo-a' }));
  });
});

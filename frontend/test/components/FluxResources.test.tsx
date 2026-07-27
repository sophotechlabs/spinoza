import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { FluxDashboard } from '../../src/lib/types';
import FluxResources from '../../src/components/FluxResources';
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
      name: 'Appliers',
      ready: 2,
      total: 3,
      resources: [
        makeFluxResource({
          kind: 'Kustomization',
          name: 'apps',
          namespace: 'flux-system',
          ready: 'True',
        }),
        makeFluxResource({
          kind: 'Kustomization',
          name: 'infra',
          namespace: 'flux-system',
          ready: 'True',
        }),
        makeFluxResource({
          kind: 'HelmRelease',
          name: 'podinfo',
          namespace: 'flux-system',
          ready: 'False',
          message: 'install failed',
        }),
      ],
    },
    {
      name: 'Sources',
      ready: 1,
      total: 1,
      resources: [
        makeFluxResource({
          kind: 'GitRepository',
          name: 'repo',
          namespace: 'flux-system',
          ready: 'True',
        }),
      ],
    },
  ],
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('FluxResources', () => {
  it('renders per-kind tiles with counts and badges', async () => {
    stubFlux(dashboard);
    render(<FluxResources onSelect={vi.fn()} />);
    expect(await screen.findByText('Kustomization')).toBeInTheDocument();
    expect(screen.getByText('HelmRelease')).toBeInTheDocument();
    expect(screen.getByText('GitRepository')).toBeInTheDocument();
    expect(screen.getByText('Bucket')).toBeInTheDocument();
    expect(screen.getByText('Appliers')).toBeInTheDocument();
    expect(screen.getByText('Image Automation')).toBeInTheDocument();
    expect(screen.getByText('2/2 ready')).toBeInTheDocument();
    expect(screen.getByText('0/1 ready')).toBeInTheDocument();
    expect(screen.getAllByText('no resources').length).toBeGreaterThan(0);
  });

  it('shows the error message when the fetch fails', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('down')));
    render(<FluxResources onSelect={vi.fn()} />);
    expect(await screen.findByText('down')).toBeInTheDocument();
  });

  it('drills into a kind list and back to the tiles', async () => {
    stubFlux(dashboard);
    render(<FluxResources onSelect={vi.fn()} />);
    await userEvent.click(await screen.findByRole('button', { name: /Kustomization/ }));
    expect(await screen.findByText(/2 resources/)).toBeInTheDocument();
    expect(screen.getByText('apps')).toBeInTheDocument();
    expect(screen.getByText('infra')).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /Flux Resources/ }));
    expect(await screen.findByText('Appliers')).toBeInTheDocument();
  });
  it('reports the clicked row in a kind list to the caller', async () => {
    stubFlux(dashboard);
    const onSelect = vi.fn();
    render(<FluxResources onSelect={onSelect} />);
    await userEvent.click(await screen.findByRole('button', { name: /Kustomization/ }));

    await userEvent.click(await screen.findByRole('button', { name: /apps/ }));

    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ name: 'apps' }));
  });
});

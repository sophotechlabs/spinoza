import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ArgoApps from '../../src/components/ArgoApps';
import type { ArgoApp } from '../../src/lib/types';

function makeApp(name: string, extra: Partial<ArgoApp> = {}): ArgoApp {
  return {
    kind: 'Application',
    group: 'argoproj.io',
    version: 'v1alpha1',
    resource: 'applications',
    name,
    namespace: 'argocd',
    project: 'default',
    sync: 'Synced',
    health: 'Healthy',
    revision: 'abc1234',
    repo: 'https://git/apps',
    path: `apps/${name}`,
    destination: 'in-cluster shop',
    message: '',
    createdAt: '2026-08-17T09:00:00Z',
    ...extra,
  };
}

function stub(body: unknown, ok = true) {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve({ ok, status: ok ? 200 : 500, json: () => Promise.resolve(body) })),
  );
}

function show(apps: ArgoApp[], extra: Record<string, unknown> = {}) {
  const onSelect = vi.fn();
  stub({ apps, applicationSets: [], ...extra });
  render(<ArgoApps onSelect={onSelect} />);
  return { onSelect };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ArgoApps', () => {
  it('says it is loading before the first answer', () => {
    stub({ apps: [] });

    render(<ArgoApps onSelect={vi.fn()} />);

    expect(screen.getByText(/Loading Argo CD/)).toBeInTheDocument();
  });

  it('lists an application with its sync and health', async () => {
    show([makeApp('web')]);

    expect(await screen.findByText('web')).toBeInTheDocument();
    expect(screen.getByText('Synced')).toBeInTheDocument();
    expect(screen.getByText('Healthy')).toBeInTheDocument();
    expect(screen.getByText('in-cluster shop')).toBeInTheDocument();
  });

  it('indents an app of apps under its parent', async () => {
    show([makeApp('root'), makeApp('web', { owner: 'root' })]);

    const child = await screen.findByText('web');
    const parent = screen.getByText('root');
    expect(parent).toHaveStyle({ paddingLeft: '0px' });
    expect(child).toHaveStyle({ paddingLeft: '16px' });
  });

  it('opens the application object when a row is clicked', async () => {
    const user = userEvent.setup();
    const { onSelect } = show([makeApp('web')]);
    await screen.findByText('web');

    await user.click(screen.getByText('web'));

    expect(onSelect).toHaveBeenCalledWith({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      namespace: 'argocd',
      name: 'web',
    });
  });

  it('says when the cluster has no applications', async () => {
    show([]);

    expect(await screen.findByText('No applications on this cluster.')).toBeInTheDocument();
  });

  it('marks a degraded application', async () => {
    show([makeApp('web', { health: 'Degraded', sync: 'OutOfSync' })]);

    expect((await screen.findByText('Degraded')).className).toContain('text-error');
    expect(screen.getByText('OutOfSync').className).toContain('text-warn');
  });

  it('leaves a dash where an application says nothing', async () => {
    show([makeApp('web', { sync: '', health: '', revision: '', destination: '' })]);

    await screen.findByText('web');
    expect(screen.getAllByText('-').length).toBeGreaterThan(0);
  });

  it('carries the health message as the row tooltip', async () => {
    show([makeApp('web', { message: 'pod crashlooping' })]);

    await screen.findByText('web');
    expect(screen.getByTitle('pod crashlooping')).toBeInTheDocument();
  });

  it('surfaces a partial failure beside the list', async () => {
    show([makeApp('web')], { error: 'applicationsets is forbidden' });

    expect(await screen.findByText('applicationsets is forbidden')).toBeInTheDocument();
    expect(screen.getByText('web')).toBeInTheDocument();
  });

  it('says why nothing could be read', async () => {
    stub({ message: 'spinoza has no cluster' }, false);

    render(<ArgoApps onSelect={vi.fn()} />);

    await waitFor(() => {
      expect(screen.getByText(/could not be read/)).toBeInTheDocument();
    });
  });

  it('marks a progressing application without alarm', async () => {
    show([makeApp('web', { health: 'Progressing' })]);

    expect((await screen.findByText('Progressing')).className).toContain('text-warn');
  });
});

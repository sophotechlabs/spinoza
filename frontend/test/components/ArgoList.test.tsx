import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ArgoList from '../../src/components/ArgoList';
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

function show(body: Record<string, unknown>) {
  const onSelect = vi.fn();
  stub({ apps: [], applicationSets: [], projects: [], ...body });
  render(<ArgoList onSelect={onSelect} />);
  return { onSelect };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ArgoList', () => {
  it('says it is loading before the first answer', () => {
    stub({ apps: [] });

    render(<ArgoList onSelect={vi.fn()} />);

    expect(screen.getByText('Loading Argo CD resources')).toBeInTheDocument();
  });

  it('groups every Argo kind under its own heading', async () => {
    show({
      apps: [makeApp('web')],
      applicationSets: [
        makeApp('shops', { kind: 'ApplicationSet', resource: 'applicationsets', sync: '' }),
      ],
      projects: [
        makeApp('default', {
          kind: 'AppProject',
          resource: 'appprojects',
          sync: '',
          health: '',
          project: '',
          destination: '',
          revision: '',
        }),
      ],
    });

    expect(await screen.findByText('Applications')).toBeInTheDocument();
    expect(screen.getByText('ApplicationSets')).toBeInTheDocument();
    expect(screen.getByText('Projects')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'web' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'shops' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'default' })).toBeInTheDocument();
  });

  it('leaves out a kind the cluster has none of', async () => {
    show({ apps: [makeApp('web')] });

    await screen.findByText('Applications');

    expect(screen.queryByText('ApplicationSets')).not.toBeInTheDocument();
    expect(screen.queryByText('Projects')).not.toBeInTheDocument();
  });

  it('dashes out the fields a project does not carry', async () => {
    show({
      projects: [
        makeApp('default', {
          kind: 'AppProject',
          resource: 'appprojects',
          sync: '',
          health: '',
          project: '',
          destination: '',
          revision: '',
        }),
      ],
    });

    await screen.findByText('Projects');
    const row = screen.getByRole('button', { name: 'default' }).closest('tr');

    expect(row?.textContent).toContain('-');
  });

  it('opens the object behind a row', async () => {
    const user = userEvent.setup();
    const { onSelect } = show({ apps: [makeApp('web')] });

    await user.click(await screen.findByRole('button', { name: 'web' }));

    expect(onSelect).toHaveBeenCalledWith({
      group: 'argoproj.io',
      version: 'v1alpha1',
      resource: 'applications',
      namespace: 'argocd',
      name: 'web',
    });
  });

  it('says so when the cluster has no Argo resources at all', async () => {
    show({});

    expect(await screen.findByText('No Argo CD resources on this cluster.')).toBeInTheDocument();
  });

  it('reports a partial read above the rows it did get', async () => {
    show({ apps: [makeApp('web')], error: 'appprojects: forbidden' });

    expect(await screen.findByText(/appprojects: forbidden/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'web' })).toBeInTheDocument();
  });

  it('shows the request failure when nothing has arrived', async () => {
    stub({ message: 'argo is unreachable' }, false);

    render(<ArgoList onSelect={vi.fn()} />);

    expect(await screen.findByText(/argo is unreachable/)).toBeInTheDocument();
  });
});

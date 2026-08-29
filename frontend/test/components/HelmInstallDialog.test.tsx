import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import HelmInstallDialog from '../../src/components/HelmInstallDialog';
import { useToastsStore } from '../../src/store/toasts';
import { useContextsStore } from '../../src/store/contexts';
import { useNamespaceStore } from '../../src/store/namespace';
import { useHelmAccessStore } from '../../src/store/helmAccess';

vi.mock('../../src/lib/monaco', () => ({
  defineEditorTheme: vi.fn(),
}));

vi.mock('@monaco-editor/react', () => ({
  default: ({
    value,
    onChange,
    options,
  }: {
    value: string;
    onChange: (next: string | undefined) => void;
    options: { readOnly: boolean };
  }) => (
    <textarea
      aria-label="yaml"
      readOnly={options.readOnly}
      value={value}
      onChange={(event) => {
        onChange(event.target.value);
      }}
    />
  ),
  DiffEditor: ({ original, modified }: { original: string; modified: string }) => (
    <div data-testid="manifest-diff" data-original={original} data-modified={modified} />
  ),
}));

const searchPayload = {
  query: 'podinfo',
  hits: [
    {
      chart: 'podinfo',
      version: '6.15.1',
      description: 'a tiny greeter',
      repo: 'podinfo',
      url: 'https://stefanprodan.github.io/podinfo',
    },
    {
      chart: 'podinfo-extras',
      version: '1.0.0',
      url: 'https://example.com/charts',
    },
  ],
};

const versionsPayload = {
  chart: 'podinfo',
  repos: [
    {
      name: 'podinfo',
      url: 'https://stefanprodan.github.io/podinfo',
      versions: ['6.15.1', '6.14.0'],
    },
  ],
};

const nginxPayload = {
  hits: [
    {
      chart: 'nginx',
      version: '1.2.3',
      repo: 'bitnami',
      url: 'https://charts.bitnami.com/bitnami',
      versions: ['1.2.3'],
    },
  ],
};

interface Stubs {
  refused?: { capability: string; reason: string }[];
  search?: unknown;
  searchStatus?: number;
  versions?: unknown;
  values?: unknown;
  valuesStatus?: number;
  installStatus?: number;
  installBody?: unknown;
}

function stub(options: Stubs = {}) {
  const calls: { url: string; method: string; body: string }[] = [];
  const fetchMock = vi.fn((url: string, init?: { method?: string; body?: string }) => {
    calls.push({ url, method: init?.method ?? 'GET', body: init?.body ?? '' });
    if (url.startsWith('/api/helm/charts')) {
      const status = options.searchStatus ?? 200;
      return Promise.resolve({
        ok: status === 200,
        status,
        json: () => Promise.resolve(options.search ?? searchPayload),
      });
    }
    if (url.startsWith('/api/helm/access')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ refused: options.refused ?? [] }),
      });
    }
    if (url.startsWith('/api/helm/versions')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve(options.versions ?? versionsPayload),
      });
    }
    if (url.startsWith('/api/helm/values')) {
      const status = options.valuesStatus ?? 200;
      return Promise.resolve({
        ok: status === 200,
        status,
        json: () =>
          Promise.resolve(
            options.values ?? { chart: 'podinfo', version: '6.15.1', values: 'replicaCount: 1\n' },
          ),
      });
    }
    const status = options.installStatus ?? 200;
    return Promise.resolve({
      ok: status === 200,
      status,
      json: () =>
        Promise.resolve(options.installBody ?? { action: 'install', message: 'installed' }),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

function renderDialog() {
  const onClose = vi.fn();
  const onInstalled = vi.fn();
  render(<HelmInstallDialog namespace="demo" onClose={onClose} onInstalled={onInstalled} />);
  return { onClose, onInstalled };
}

async function search(user: ReturnType<typeof userEvent.setup>, text = 'podinfo') {
  await user.type(screen.getByLabelText('Search charts'), text);
  return screen.findByRole('button', { name: /^podinfo 6.15.1 from/ });
}

async function reachTheForm(user: ReturnType<typeof userEvent.setup>) {
  const hit = await search(user);
  await user.click(hit);
  await screen.findByLabelText('Chart version');
}

const showModal = vi.fn(function showModal(this: HTMLDialogElement) {
  this.open = true;
});
const close = vi.fn(function close(this: HTMLDialogElement) {
  this.open = false;
});

beforeEach(() => {
  useToastsStore.getState().clear();
  useHelmAccessStore.setState({ answers: {} });
  useNamespaceStore.getState().offer(['default', 'demo']);
  showModal.mockClear();
  close.mockClear();
  HTMLDialogElement.prototype.showModal = showModal;
  HTMLDialogElement.prototype.close = close;
});

afterEach(() => {
  vi.unstubAllGlobals();
  useContextsStore.getState().reset();
  useNamespaceStore.getState().offer([]);
});

describe('HelmInstallDialog', () => {
  it('opens as a modal and waits for a search', () => {
    stub();

    renderDialog();

    expect(showModal).toHaveBeenCalled();
    expect(screen.getByText('type part of a chart name')).toBeInTheDocument();
  });

  it('searches every repository and shows what came back', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDialog();

    await search(user);

    expect(screen.getByText('a tiny greeter')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^podinfo-extras/ })).toBeInTheDocument();
    expect(calls.some((call) => call.url.includes('/api/helm/charts?query=podinfo'))).toBe(true);
  });

  it('says when nothing matches', async () => {
    const user = userEvent.setup();
    stub({ search: { query: 'nope', hits: [] } });
    renderDialog();

    await user.type(screen.getByLabelText('Search charts'), 'nope');

    expect(
      await screen.findByText('no configured repository offers a chart by that name'),
    ).toBeInTheDocument();
  });

  it('passes on what the backend said about a repository it could not read', async () => {
    const user = userEvent.setup();
    stub({ search: { query: 'podinfo', hits: [], error: 'bitnami: 404' } });
    renderDialog();

    await user.type(screen.getByLabelText('Search charts'), 'podinfo');

    expect(await screen.findByText('bitnami: 404')).toBeInTheDocument();
  });

  it('says when the search itself failed', async () => {
    const user = userEvent.setup();
    stub({ searchStatus: 500, search: { message: 'the index is unreachable' } });
    renderDialog();

    await user.type(screen.getByLabelText('Search charts'), 'podinfo');

    expect(await screen.findByText(/the index is unreachable/)).toBeInTheDocument();
  });

  it('says when the search was capped', async () => {
    const user = userEvent.setup();
    stub({ search: { ...searchPayload, truncated: true } });
    renderDialog();

    await search(user);

    expect(screen.getByText(/Only the first matches are shown/)).toBeInTheDocument();
  });

  it('takes the release name and the newest version from the chart that was picked', async () => {
    const user = userEvent.setup();
    stub();
    renderDialog();

    await reachTheForm(user);

    expect(screen.getByLabelText('Release name')).toHaveValue('podinfo');
    expect(screen.getByLabelText('Chart version')).toHaveValue('0:6.15.1');
    expect(screen.getByLabelText('Namespace')).toHaveValue('demo');
  });

  it('goes back to the search when the chart is changed', async () => {
    const user = userEvent.setup();
    stub();
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Change chart' }));

    expect(screen.getByLabelText('Search charts')).toBeInTheDocument();
  });

  it('loads the chart defaults into the editor when asked', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Load the chart defaults' }));

    await waitFor(() => {
      expect(screen.getByLabelText('yaml')).toHaveValue('replicaCount: 1\n');
    });
    const asked = calls.find((call) => call.url.startsWith('/api/helm/values'));
    expect(asked?.url).toContain('chart=podinfo');
    expect(asked?.url).toContain('version=6.15.1');
  });

  it('says when the chart defaults could not be read', async () => {
    const user = userEvent.setup();
    stub({ valuesStatus: 500, values: { message: 'chart not found' } });
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Load the chart defaults' }));

    expect(await screen.findByText(/chart not found/)).toBeInTheDocument();
  });

  it('previews the render before installing anything', async () => {
    const user = userEvent.setup();
    const calls = stub({
      installBody: { action: 'install', dryRun: true, manifest: 'kind: Service\n' },
    });
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Preview' }));

    const diff = await screen.findByTestId('manifest-diff');
    expect(diff).toHaveAttribute('data-modified', 'kind: Service\n');
    const asked = calls.find((call) => call.url.startsWith('/api/helm/install'));
    expect(asked?.url).toContain('dryRun=true');
    expect(JSON.parse(asked?.body ?? '{}')).toMatchObject({
      namespace: 'demo',
      name: 'podinfo',
      chart: 'podinfo',
      repo: 'https://stefanprodan.github.io/podinfo',
      version: '6.15.1',
      createNamespace: false,
    });
  });

  it('installs what was previewed', async () => {
    const user = userEvent.setup();
    const calls = stub();
    const { onClose, onInstalled } = renderDialog();
    await reachTheForm(user);
    await user.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByTestId('manifest-diff');

    await user.click(await screen.findByRole('button', { name: 'Install podinfo' }));

    await waitFor(() => {
      expect(onInstalled).toHaveBeenCalled();
    });
    expect(onClose).toHaveBeenCalled();
    const install = calls.filter((call) => call.url.startsWith('/api/helm/install'));
    expect(install).toHaveLength(2);
    expect(install[1].url).not.toContain('dryRun');
  });

  it('asks the namespace to be created when that was ticked', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDialog();
    await reachTheForm(user);
    await user.clear(screen.getByLabelText('Namespace'));
    await user.type(screen.getByLabelText('Namespace'), 'fresh');
    await user.click(screen.getByLabelText('Create the namespace'));

    await user.click(screen.getByRole('button', { name: 'Preview' }));

    const asked = calls.find((call) => call.url.startsWith('/api/helm/install'));
    expect(JSON.parse(asked?.body ?? '{}')).toMatchObject({
      namespace: 'fresh',
      createNamespace: true,
    });
  });

  it('reports an install the backend refused', async () => {
    const user = userEvent.setup();
    stub({ installStatus: 500, installBody: { message: 'cannot re-use a name' } });
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Preview' }));

    expect(await screen.findByText(/cannot re-use a name/)).toBeInTheDocument();
  });

  it('goes back from the preview to the values', async () => {
    const user = userEvent.setup();
    stub();
    renderDialog();
    await reachTheForm(user);
    await user.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByTestId('manifest-diff');

    await user.click(screen.getByRole('button', { name: 'Back' }));

    expect(screen.getByLabelText('yaml')).toBeInTheDocument();
  });

  it('types the name before installing on a protected cluster', async () => {
    const user = userEvent.setup();
    const calls = stub();
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'prod' },
      kubeconfigs: [],
      protection: 'protected',
    });
    renderDialog();
    await reachTheForm(user);
    await user.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByTestId('manifest-diff');

    await user.click(await screen.findByRole('button', { name: 'Install podinfo' }));

    const confirm = await screen.findByLabelText('Name');
    await user.type(confirm, 'podinfo');
    await user.click(screen.getByRole('button', { name: 'Confirm' }));

    await waitFor(() => {
      const install = calls.filter((call) => call.url.startsWith('/api/helm/install'));
      expect(install[1].url).toContain('confirm=podinfo');
    });
  });

  it('closes when the dialog is dismissed', async () => {
    const user = userEvent.setup();
    stub();
    const { onClose } = renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(onClose).toHaveBeenCalled();
  });

  it('closes from the corner button', async () => {
    const user = userEvent.setup();
    stub();
    const { onClose } = renderDialog();

    await user.click(screen.getByRole('button', { name: 'Close the install dialog' }));

    expect(onClose).toHaveBeenCalled();
  });

  it('says when the version lookup failed', async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn((url: string) => {
      if (url.startsWith('/api/helm/charts')) {
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve(searchPayload),
        });
      }
      return Promise.resolve({
        ok: false,
        status: 500,
        json: () => Promise.resolve({ message: 'no repository knows that chart' }),
      });
    });
    vi.stubGlobal('fetch', fetchMock);
    renderDialog();

    const hit = await search(user);
    await user.click(hit);

    expect(await screen.findByText(/no repository knows that chart/)).toBeInTheDocument();
  });
});

describe('the corners of the install dialog', () => {
  it('offers nothing to install when no repository has the chart', async () => {
    const user = userEvent.setup();
    stub({ versions: { chart: 'podinfo', repos: [] } });
    renderDialog();
    const hit = await search(user);

    await user.click(hit);

    expect(await screen.findByRole('button', { name: 'Preview' })).toBeDisabled();
  });

  it('names a repository by its url when it has none', async () => {
    const user = userEvent.setup();
    stub({
      versions: { chart: 'podinfo', repos: [{ url: 'https://example.com', versions: ['6.15.1'] }] },
    });
    renderDialog();

    await reachTheForm(user);

    expect(screen.getByRole('group', { name: 'https://example.com' })).toBeInTheDocument();
  });

  it('takes another version', async () => {
    const user = userEvent.setup();
    stub();
    renderDialog();
    await reachTheForm(user);

    await user.selectOptions(screen.getByLabelText('Chart version'), '0:6.14.0');

    expect(screen.getByLabelText('Chart version')).toHaveValue('0:6.14.0');
  });

  it('takes another release name', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDialog();
    await reachTheForm(user);

    await user.clear(screen.getByLabelText('Release name'));
    await user.type(screen.getByLabelText('Release name'), 'greeter');
    await user.click(screen.getByRole('button', { name: 'Preview' }));

    const asked = calls.find((call) => call.url.startsWith('/api/helm/install'));
    expect(JSON.parse(asked?.body ?? '{}')).toMatchObject({ name: 'greeter' });
  });

  it('reports an install that failed after the preview', async () => {
    const user = userEvent.setup();
    let installs = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/helm/charts')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(searchPayload),
          });
        }
        if (url.startsWith('/api/helm/access')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ refused: [] }),
          });
        }
        if (url.startsWith('/api/helm/versions')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(versionsPayload),
          });
        }
        installs += 1;
        if (installs === 1) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () =>
              Promise.resolve({ action: 'install', dryRun: true, manifest: 'kind: Service\n' }),
          });
        }
        return Promise.resolve({
          ok: false,
          status: 500,
          json: () => Promise.resolve({ message: 'cannot re-use a name that is still in use' }),
        });
      }),
    );
    renderDialog();
    await reachTheForm(user);
    await user.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByTestId('manifest-diff');

    await user.click(await screen.findByRole('button', { name: 'Install podinfo' }));

    expect(await screen.findByText(/cannot re-use a name/)).toBeInTheDocument();
  });

  it('lets the typed confirmation be called off', async () => {
    const user = userEvent.setup();
    const calls = stub();
    useContextsStore.getState().setList({
      current: { kubeconfig: '', name: 'prod' },
      kubeconfigs: [],
      protection: 'protected',
    });
    renderDialog();
    await reachTheForm(user);
    await user.click(screen.getByRole('button', { name: 'Preview' }));
    await screen.findByTestId('manifest-diff');
    await user.click(await screen.findByRole('button', { name: 'Install podinfo' }));

    await user.click(screen.getByRole('button', { name: 'Cancel' }));

    expect(screen.queryByLabelText('Name')).not.toBeInTheDocument();
    expect(calls.filter((call) => call.url.startsWith('/api/helm/install'))).toHaveLength(1);
  });

  it('falls back to a plain reason when the failure carries none', async () => {
    const user = userEvent.setup();
    const rejectNonError = vi.fn<() => Promise<never>>().mockRejectedValue('nope');
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/helm/charts')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(searchPayload),
          });
        }
        if (url.startsWith('/api/helm/access')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ refused: [] }),
          });
        }
        if (url.startsWith('/api/helm/versions')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(versionsPayload),
          });
        }
        return rejectNonError();
      }),
    );
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Load the chart defaults' }));

    expect(await screen.findByText('the chart values could not be read')).toBeInTheDocument();
  });

  it('drops a search that lands after it is gone', async () => {
    const user = userEvent.setup();
    const settle: { resolve: ((value: unknown) => void) | null } = { resolve: null };
    const fetchMock = vi.fn(
      () =>
        new Promise((resolve) => {
          settle.resolve = resolve;
        }),
    );
    vi.stubGlobal('fetch', fetchMock);
    const view = render(
      <HelmInstallDialog namespace="demo" onClose={vi.fn()} onInstalled={vi.fn()} />,
    );
    await user.type(screen.getByLabelText('Search charts'), 'podinfo');
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });

    view.unmount();
    settle.resolve?.({ ok: true, status: 200, json: () => Promise.resolve(searchPayload) });

    expect(screen.queryByText('a tiny greeter')).not.toBeInTheDocument();
  });

  it('drops a version lookup that lands after it is gone', async () => {
    const user = userEvent.setup();
    const settle: { resolve: ((value: unknown) => void) | null } = { resolve: null };
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/helm/charts')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(searchPayload),
          });
        }
        return new Promise((resolve) => {
          settle.resolve = resolve;
        });
      }),
    );
    const view = render(
      <HelmInstallDialog namespace="demo" onClose={vi.fn()} onInstalled={vi.fn()} />,
    );
    await user.click(await search(user));

    view.unmount();
    settle.resolve?.({ ok: true, status: 200, json: () => Promise.resolve(versionsPayload) });

    expect(screen.queryByLabelText('Chart version')).not.toBeInTheDocument();
  });
});

describe('HelmInstallDialog and a search that was left behind', () => {
  it('drops the hits for a query that has moved on', async () => {
    const user = userEvent.setup();
    let answerFirst: (body: unknown) => void = () => undefined;
    const held = new Promise((resolve) => {
      answerFirst = resolve;
    });
    let asked = 0;
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/helm/charts')) {
          asked += 1;
          if (asked === 1) {
            return held;
          }
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve(nginxPayload),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () => Promise.resolve({ refused: [] }),
        });
      }),
    );
    renderDialog();

    await user.type(screen.getByLabelText('Search charts'), 'pod');
    await waitFor(() => {
      expect(asked).toBe(1);
    });
    await user.type(screen.getByLabelText('Search charts'), 'x');
    await screen.findByRole('button', { name: /^nginx/ });
    answerFirst({ ok: true, status: 200, json: () => Promise.resolve(searchPayload) });

    await waitFor(() => {
      expect(screen.getByRole('button', { name: /^nginx/ })).toBeInTheDocument();
    });
    expect(screen.queryByRole('button', { name: /^podinfo/ })).not.toBeInTheDocument();
  });
});

describe('HelmInstallDialog when the cluster refuses', () => {
  it('holds back Install and says why', async () => {
    const user = userEvent.setup();
    stub({ refused: [{ capability: 'install', reason: 'no creating secrets in demo' }] });
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Preview' }));
    const install = await screen.findByRole('button', { name: 'Install podinfo' });

    await waitFor(() => {
      expect(install).toBeDisabled();
    });
    expect(install).toHaveAttribute('title', 'no creating secrets in demo');
  });

  it('still previews what would be installed', async () => {
    const user = userEvent.setup();
    const calls = stub({
      refused: [{ capability: 'install', reason: 'no creating secrets in demo' }],
      installBody: { action: 'install', dryRun: true, manifest: 'kind: Service\n' },
    });
    renderDialog();
    await reachTheForm(user);

    const preview = screen.getByRole('button', { name: 'Preview' });
    expect(preview).toBeEnabled();
    await user.click(preview);

    expect(await screen.findByTestId('manifest-diff')).toBeInTheDocument();
    expect(calls.some((call) => call.url.includes('dryRun=true'))).toBe(true);
  });

  it('leaves Install alone when nothing is refused', async () => {
    const user = userEvent.setup();
    stub({ installBody: { action: 'install', dryRun: true, manifest: 'kind: Service\n' } });
    renderDialog();
    await reachTheForm(user);

    await user.click(screen.getByRole('button', { name: 'Preview' }));
    const install = await screen.findByRole('button', { name: 'Install podinfo' });

    await waitFor(() => {
      expect(install).toBeEnabled();
    });
    expect(install).toHaveAttribute('title', 'Install podinfo into demo');
  });

  it('asks about the namespace it would install into', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDialog();
    await reachTheForm(user);

    await waitFor(() => {
      expect(calls.some((call) => call.url === '/api/helm/access?namespace=demo')).toBe(true);
    });
  });

  it('asks again when the namespace is changed', async () => {
    const user = userEvent.setup();
    const calls = stub();
    renderDialog();
    await reachTheForm(user);
    await waitFor(() => {
      expect(calls.some((call) => call.url === '/api/helm/access?namespace=demo')).toBe(true);
    });

    await user.clear(screen.getByLabelText('Namespace'));
    await user.type(screen.getByLabelText('Namespace'), 'prod');

    await waitFor(() => {
      expect(calls.some((call) => call.url === '/api/helm/access?namespace=prod')).toBe(true);
    });
  });
});

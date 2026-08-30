import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import Checks from '../../src/components/Checks';
import { useSettingsStore } from '../../src/store/settings';
import type { CheckFinding, CheckGroup, CheckObject, CheckReport } from '../../src/lib/types';

const OBJECTS: CheckObject[] = [
  {
    group: 'apps',
    version: 'v1',
    resource: 'deployments',
    namespace: 'apps',
    name: 'api',
    kind: 'Deployment',
  },
];

function bodyIn(init?: RequestInit): unknown {
  if (typeof init?.body !== 'string') {
    return null;
  }
  return JSON.parse(init.body);
}

function finding(extra: Partial<CheckFinding> = {}): CheckFinding {
  return { ref: 0, detail: 'securityContext.privileged is true', ...extra };
}

function group(extra: Partial<CheckGroup> = {}): CheckGroup {
  const findings = extra.findings ?? [finding()];
  return {
    id: 'privileged-containers',
    title: 'Privileged containers',
    category: 'security',
    severity: 'high',
    wrong: 'a privileged container holds every capability on the node',
    remedy: 'remove securityContext.privileged',
    total: findings.length,
    ...extra,
    findings,
  };
}

function answers(report: Partial<CheckReport>) {
  const calls: { url: string; method: string; body: unknown }[] = [];
  const fetchMock = vi.fn((url: string, init?: RequestInit) => {
    calls.push({
      url,
      method: init?.method ?? 'GET',
      body: bodyIn(init),
    });
    if (url.startsWith('/api/checks/mutes')) {
      return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ mutes: [] }) });
    }
    if (url.startsWith('/api/checks/baseline')) {
      return Promise.resolve({
        ok: true,
        status: 200,
        json: () => Promise.resolve({ takenAt: '2026-08-30T00:00:00Z' }),
      });
    }
    return Promise.resolve({
      ok: true,
      status: 200,
      json: () =>
        Promise.resolve({
          groups: [],
          objects: OBJECTS,
          namespaces: [],
          scanned: 1,
          ...report,
        }),
    });
  });
  vi.stubGlobal('fetch', fetchMock);
  render(<Checks onOpen={vi.fn()} />);
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  useSettingsStore.setState({
    checksDisabled: [],
    checksSkipNamespaces: [],
    checksMinSeverity: '',
    checksWholeCluster: true,
    checksEveryKind: false,
    checksNamespace: '',
    checksOnlyNew: false,
    checksShowMuted: false,
    checkRules: '',
  });
});

describe('muting a finding', () => {
  it('sends what was decided and why', async () => {
    const calls = answers({ groups: [group()] });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Mute Deployment/ }));
    await userEvent.type(
      screen.getByRole('textbox', { name: 'Why this one is being muted' }),
      'it is the node agent',
    );
    await userEvent.click(screen.getByRole('button', { name: 'Mute this one' }));

    await waitFor(() => {
      expect(calls.some((call) => call.url === '/api/checks/mutes')).toBe(true);
    });
    const sent = calls.find((call) => call.url === '/api/checks/mutes');
    expect(sent?.body).toEqual({
      check: 'privileged-containers',
      reason: 'it is the node agent',
      ref: 'apps/v1/deployments/apps/api',
    });
  });

  it('can silence the whole namespace instead of the one object', async () => {
    const calls = answers({ groups: [group()] });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Mute Deployment/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Mute in apps' }));

    await waitFor(() => {
      expect(calls.some((call) => call.url === '/api/checks/mutes')).toBe(true);
    });
    expect(calls.find((call) => call.url === '/api/checks/mutes')?.body).toEqual({
      check: 'privileged-containers',
      reason: '',
      namespace: 'apps',
    });
  });

  it('shows the reason a muted finding was given, and offers to undo it', async () => {
    const calls = answers({
      groups: [
        group({
          muted: 1,
          findings: [finding({ muted: true, reason: 'the node agent needs it' })],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText(/the node agent needs it/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: /^Unmute Deployment/ }));

    await waitFor(() => {
      expect(
        calls.some((call) => call.url === '/api/checks/mutes' && call.method === 'DELETE'),
      ).toBe(true);
    });
  });

  it('undoes a namespace mute at the namespace, not the object', async () => {
    const calls = answers({
      groups: [
        group({
          muted: 1,
          findings: [finding({ muted: true, mutedBy: 'namespace', reason: 'prod is exempt' })],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Unmute Deployment/ }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'DELETE')).toBe(true);
    });
    expect(calls.find((call) => call.method === 'DELETE')?.body).toEqual({
      check: 'privileged-containers',
      namespace: 'apps',
    });
  });

  it('undoes a whole-check mute without naming an object', async () => {
    const calls = answers({
      groups: [group({ muted: 1, findings: [finding({ muted: true, mutedBy: 'check' })] })],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Unmute Deployment/ }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'DELETE')).toBe(true);
    });
    expect(calls.find((call) => call.method === 'DELETE')?.body).toEqual({
      check: 'privileged-containers',
    });
  });

  it('says so when an unmute could not be saved', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        if (url.startsWith('/api/checks/mutes') && init?.method === 'DELETE') {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the settings file is read only' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              groups: [group({ muted: 1, findings: [finding({ muted: true })] })],
              objects: OBJECTS,
              namespaces: [],
              scanned: 1,
            }),
        });
      }),
    );
    render(<Checks onOpen={vi.fn()} />);

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Unmute Deployment/ }));

    expect(await screen.findByText('the settings file is read only')).toBeInTheDocument();
  });

  it('says how many a check is silent about', async () => {
    answers({ groups: [group({ muted: 4 })] });

    expect(await screen.findByText('4 muted')).toBeInTheDocument();
  });

  it('says so when a mute could not be saved', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/mutes')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the settings file is read only' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ groups: [group()], objects: OBJECTS, namespaces: [], scanned: 1 }),
        });
      }),
    );
    render(<Checks onOpen={vi.fn()} />);

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByRole('button', { name: /^Mute Deployment/ }));
    await userEvent.click(screen.getByRole('button', { name: 'Mute this one' }));

    expect(await screen.findByText('the settings file is read only')).toBeInTheDocument();
  });
});

describe('the baseline', () => {
  it('says there is nothing to compare against until one is taken', async () => {
    answers({ groups: [group()] });

    expect(await screen.findByText(/No baseline taken/)).toBeInTheDocument();
  });

  it('takes one when asked', async () => {
    const calls = answers({ groups: [group()] });

    await userEvent.click(await screen.findByRole('button', { name: 'Take a baseline' }));

    await waitFor(() => {
      expect(
        calls.some((call) => call.url === '/api/checks/baseline' && call.method === 'POST'),
      ).toBe(true);
    });
  });

  it('forgets one when asked', async () => {
    const calls = answers({ groups: [group()], baseline: '2026-08-29T00:00:00Z' });

    await userEvent.click(await screen.findByRole('button', { name: 'forget it' }));

    await waitFor(() => {
      expect(
        calls.some((call) => call.url === '/api/checks/baseline' && call.method === 'DELETE'),
      ).toBe(true);
    });
  });

  it('says what moved since the baseline', async () => {
    answers({
      groups: [group({ baselined: true, new: 2, fixed: 3 })],
      baseline: '2026-08-29T00:00:00Z',
    });

    expect(await screen.findByText('2 new · 3 fixed')).toBeInTheDocument();
    expect(screen.getByText(/Comparing against 2026-08-29/)).toBeInTheDocument();
  });

  it('says when a check was not in the baseline rather than calling it all new', async () => {
    answers({ groups: [group({ baselined: false })], baseline: '2026-08-29T00:00:00Z' });

    expect(await screen.findByText('not in the baseline')).toBeInTheDocument();
  });

  it('marks the findings that were not there before', async () => {
    answers({
      groups: [group({ baselined: true, new: 1, findings: [finding({ new: true })] })],
      baseline: '2026-08-29T00:00:00Z',
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('new')).toBeInTheDocument();
  });

  it('asks for only what is new', async () => {
    const calls = answers({ groups: [group()], baseline: '2026-08-29T00:00:00Z' });

    await userEvent.click(await screen.findByLabelText('Only what is new'));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('onlyNew=1'))).toBe(true);
    });
  });

  it('asks for the muted ones back', async () => {
    const calls = answers({ groups: [group()] });

    await userEvent.click(await screen.findByLabelText('Show what is muted'));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('showMuted=1'))).toBe(true);
    });
  });

  it('says so when a baseline could not be taken', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/baseline')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the disk is full' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ groups: [group()], objects: OBJECTS, namespaces: [], scanned: 1 }),
        });
      }),
    );
    render(<Checks onOpen={vi.fn()} />);

    await userEvent.click(await screen.findByRole('button', { name: 'Take a baseline' }));

    expect(await screen.findByText('the disk is full')).toBeInTheDocument();
  });
});

describe('the namespace summary', () => {
  const namespaces = [
    { namespace: 'prod', total: 40, high: 12, medium: 20, low: 8 },
    { namespace: 'staging', total: 9, high: 1, medium: 4, low: 4 },
  ];

  it('says how many namespaces carry findings', async () => {
    answers({ groups: [group()], namespaces });

    expect(await screen.findByText('2 namespaces with findings')).toBeInTheDocument();
  });

  it('narrows to one namespace when it is picked', async () => {
    const calls = answers({ groups: [group()], namespaces });

    await userEvent.click(await screen.findByText('2 namespaces with findings'));
    await userEvent.click(screen.getByRole('button', { name: 'prod' }));

    await waitFor(() => {
      expect(calls.some((call) => call.url.includes('namespace=prod'))).toBe(true);
    });
    expect(screen.getByText('Showing prod only')).toBeInTheDocument();
  });

  it('picking the same one again shows everything', async () => {
    answers({ groups: [group()], namespaces });
    useSettingsStore.setState({ checksNamespace: 'prod' });

    await userEvent.click(await screen.findByText('Showing prod only'));
    await userEvent.click(screen.getByRole('button', { name: 'prod' }));

    expect(screen.getByText('2 namespaces with findings')).toBeInTheDocument();
  });

  it('names the namespace it is counting rather than the whole cluster', async () => {
    answers({ groups: [group()], namespaces });
    useSettingsStore.setState({ checksNamespace: 'prod' });

    expect(await screen.findByText('1 findings in prod')).toBeInTheDocument();
  });

  it('stays out of the way when nothing has a namespace', async () => {
    answers({ groups: [group()], namespaces: [] });

    await screen.findByText(/Privileged containers/);
    expect(screen.queryByText(/namespaces with findings/)).not.toBeInTheDocument();
  });
});

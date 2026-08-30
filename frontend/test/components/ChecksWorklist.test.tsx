import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
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
  return { ref: 0, detail: 'securityContext.privileged is true', severity: 'high', ...extra };
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

  it('says which cluster a foreign baseline came from and how big each is', async () => {
    answers({
      groups: [group()],
      baseline: '2026-08-29T00:00:00Z',
      baselineFrom: 'https://10.10.0.1:6443',
      wasScanned: 396,
      scanned: 2284,
    });

    const bar = await screen.findByText(
      /Comparing against https:\/\/10\.10\.0\.1:6443, 2026-08-29/,
    );
    expect(bar).toHaveTextContent('396 workloads there, 2284 here');
    expect(bar).toHaveTextContent('counts rather than what is new');
  });

  it('counts a check on both clusters instead of calling its findings new', async () => {
    answers({
      groups: [group({ ran: true, was: 450, total: 4268 })],
      baseline: '2026-08-29T00:00:00Z',
      baselineFrom: 'https://10.10.0.1:6443',
      wasScanned: 396,
      scanned: 2284,
    });

    expect(await screen.findByText('450 there, 4268 here (1.6× per workload)')).toBeInTheDocument();
  });

  it('says a check the foreign baseline never ran was not in it', async () => {
    answers({
      groups: [group()],
      baseline: '2026-08-29T00:00:00Z',
      baselineFrom: 'https://10.10.0.1:6443',
      wasScanned: 396,
      scanned: 2284,
    });

    expect(await screen.findByText('not in the baseline')).toBeInTheDocument();
  });

  it('says a check both clusters are clean on', async () => {
    answers({
      groups: [group({ ran: true, was: 0, total: 0, findings: [] })],
      baseline: '2026-08-29T00:00:00Z',
      baselineFrom: 'https://10.10.0.1:6443',
      wasScanned: 396,
      scanned: 2284,
    });

    expect(await screen.findByText('clean on both')).toBeInTheDocument();
  });

  it('leaves a measured check out of a cross-cluster comparison too', async () => {
    answers({
      groups: [group({ measured: true, ran: true, was: 3 })],
      baseline: '2026-08-29T00:00:00Z',
      baselineFrom: 'https://10.10.0.1:6443',
      wasScanned: 396,
      scanned: 2284,
    });

    expect(await screen.findByText('measured, not compared')).toBeInTheDocument();
  });

  it('says nothing about a cluster when the baseline is this one', async () => {
    answers({ groups: [group()], baseline: '2026-08-29T00:00:00Z' });

    expect(await screen.findByText('Comparing against 2026-08-29.')).toBeInTheDocument();
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

describe('your own rules', () => {
  const onCleanup: (() => void)[] = [];

  afterEach(() => {
    while (onCleanup.length > 0) {
      onCleanup.pop()?.();
    }
  });

  function faultsAnswer(faults: unknown[]) {
    const calls: { url: string; method: string; body: unknown }[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, method: init?.method ?? 'GET', body: init?.body ?? null });
        if (url.startsWith('/api/checks/rules/faults')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ faults }),
          });
        }
        if (url.startsWith('/api/settings')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ values: {} }),
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
    return calls;
  }

  function afterSave(save: (rules: string) => Promise<void>) {
    onCleanup.push(() => {
      useSettingsStore.setState({ setCheckRules: save });
    });
  }

  async function openRules() {
    await userEvent.click(await screen.findByText('Your own rules'));
    return screen.getByRole('textbox', { name: 'Your own rules' });
  }

  it('says every rule reads when the server finds nothing wrong', async () => {
    faultsAnswer([]);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Check' }));

    expect(await screen.findByText('Every rule reads.')).toBeInTheDocument();
  });

  it('names the rule that does not compile', async () => {
    faultsAnswer([{ id: 'broken', reason: 'the expression did not compile: syntax error' }]);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Check' }));

    expect(
      await screen.findByText('broken: the expression did not compile: syntax error'),
    ).toBeInTheDocument();
  });

  it('reports a fault about the list itself without a name in front of it', async () => {
    faultsAnswer([{ id: '', reason: 'this is not a list of rules' }]);

    const box = await openRules();
    await userEvent.type(box, 'nonsense');
    await userEvent.click(screen.getByRole('button', { name: 'Check' }));

    expect(await screen.findByText('this is not a list of rules')).toBeInTheDocument();
  });

  it('refuses to save a rule list the server would refuse', async () => {
    const calls = faultsAnswer([{ id: 'broken', reason: 'the expression did not compile' }]);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(calls.some((call) => call.url.startsWith('/api/checks/rules/faults'))).toBe(true);
    });
    expect(useSettingsStore.getState().checkRules).toBe('');
  });

  it('saves a rule list that reads', async () => {
    faultsAnswer([]);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    await waitFor(() => {
      expect(useSettingsStore.getState().checkRules).toBe('x');
    });
  });

  it('says so when the rules read but could not be saved', async () => {
    faultsAnswer([]);
    const save = useSettingsStore.getState().setCheckRules;
    useSettingsStore.setState({
      setCheckRules: () => Promise.reject(new Error('the settings file is read only')),
    });
    afterSave(save);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText('the settings file is read only')).toBeInTheDocument();
  });

  it('copies the rules so they can be kept somewhere else', async () => {
    const writeText = vi.fn(() => Promise.resolve());
    vi.stubGlobal('navigator', { clipboard: { writeText } });
    faultsAnswer([]);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith('x');
    });
    expect(await screen.findByRole('button', { name: 'Copied' })).toBeInTheDocument();
  });

  it('says so when the rules could not be copied', async () => {
    vi.stubGlobal('navigator', { clipboard: { writeText: () => Promise.reject(new Error('no')) } });
    faultsAnswer([]);

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Copy' }));

    expect(await screen.findByText('the rules could not be copied')).toBeInTheDocument();
  });

  it('says so when saving could not even read the rules back', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/rules/faults')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the rules could not be read' }),
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

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));

    expect(await screen.findByText('the rules could not be read')).toBeInTheDocument();
    expect(useSettingsStore.getState().checkRules).toBe('');
  });

  it('says so when the check itself could not be run', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/rules/faults')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the rules could not be read' }),
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

    const box = await openRules();
    await userEvent.type(box, 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Check' }));

    expect(await screen.findByText('the rules could not be read')).toBeInTheDocument();
  });
});

describe('a check that reads live measurement', () => {
  it('says it is measured rather than comparing it', async () => {
    answers({
      groups: [group({ measured: true })],
      baseline: '2026-08-29T00:00:00Z',
    });

    expect(await screen.findByText('measured, not compared')).toBeInTheDocument();
  });
});

describe('what you have muted', () => {
  function withMutes(mutes: unknown[]) {
    const calls: { url: string; method: string; body: unknown }[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, method: init?.method ?? 'GET', body: bodyIn(init) });
        if (url.startsWith('/api/checks/mutes')) {
          return Promise.resolve({ ok: true, status: 200, json: () => Promise.resolve({ mutes }) });
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
    return calls;
  }

  it('reads them only once the panel is opened', async () => {
    const calls = withMutes([]);
    await screen.findByText(/Privileged containers/);

    expect(calls.some((call) => call.url === '/api/checks/mutes')).toBe(false);
    await userEvent.click(screen.getByText('What you have muted'));

    await waitFor(() => {
      expect(calls.some((call) => call.url === '/api/checks/mutes')).toBe(true);
    });
  });

  it('says so when nothing is muted', async () => {
    withMutes([]);

    await userEvent.click(await screen.findByText('What you have muted'));

    expect(
      await screen.findByText('You have not muted anything on this cluster.'),
    ).toBeInTheDocument();
  });

  it('says what each mute covers, why, and when', async () => {
    withMutes([
      {
        check: 'runs-as-root',
        namespace: 'kube-system',
        reason: 'it needs the host',
        at: '2026-08-30',
      },
      {
        check: 'privileged-containers',
        ref: 'apps/v1/deployments/apps/api',
        reason: 'known',
        at: '2026-08-29',
      },
      { check: 'image-latest', reason: 'everywhere', at: '2026-08-28' },
    ]);

    await userEvent.click(await screen.findByText('What you have muted'));

    expect(await screen.findByText(/everything in kube-system/)).toBeInTheDocument();
    expect(screen.getByText(/apps\/v1\/deployments\/apps\/api/)).toBeInTheDocument();
    expect(screen.getByText(/image-latest · everywhere/)).toBeInTheDocument();
    expect(screen.getByText('2026-08-30')).toBeInTheDocument();
  });

  it('removes one when asked', async () => {
    const calls = withMutes([
      {
        check: 'runs-as-root',
        namespace: 'kube-system',
        reason: 'it needs the host',
        at: '2026-08-30',
      },
    ]);

    await userEvent.click(await screen.findByText('What you have muted'));
    await userEvent.click(await screen.findByRole('button', { name: /^Unmute runs-as-root/ }));

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'DELETE')).toBe(true);
    });
    expect(calls.find((call) => call.method === 'DELETE')?.body).toEqual({
      check: 'runs-as-root',
      namespace: 'kube-system',
      reason: 'it needs the host',
      at: '2026-08-30',
    });
  });

  it('says so when one could not be removed', async () => {
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
        if (url.startsWith('/api/checks/mutes')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () =>
              Promise.resolve({
                mutes: [{ check: 'runs-as-root', namespace: 'kube-system', at: '2026-08-30' }],
              }),
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

    await userEvent.click(await screen.findByText('What you have muted'));
    await userEvent.click(await screen.findByRole('button', { name: /^Unmute runs-as-root/ }));

    expect(await screen.findByText('the settings file is read only')).toBeInTheDocument();
  });

  it('says so when they could not be read', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/mutes')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the settings file is unreadable' }),
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

    await userEvent.click(await screen.findByText('What you have muted'));

    expect(await screen.findByText('the settings file is unreadable')).toBeInTheDocument();
  });
});

describe('what the audit silenced itself', () => {
  it('offers no undo, because nobody decided it', async () => {
    answers({
      groups: [
        group({
          muted: 1,
          findings: [finding({ muted: true, mutedBy: 'convention', reason: 'it is read by k3s' })],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('silenced')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Unmute Deployment/ })).not.toBeInTheDocument();
    expect(screen.getByText(/it is read by k3s/)).toBeInTheDocument();
  });
});

describe('what went away since the baseline', () => {
  it('names them rather than only counting them', async () => {
    answers({
      groups: [
        group({
          baselined: true,
          fixed: 2,
          gone: ['Deployment apps/web', 'Deployment apps/worker'],
        }),
      ],
      baseline: '2026-08-29T00:00:00Z',
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));
    await userEvent.click(screen.getByText('2 that were here at the baseline and are not now'));

    expect(screen.getByText('Deployment apps/web')).toBeInTheDocument();
    expect(screen.getByText('Deployment apps/worker')).toBeInTheDocument();
  });

  it('counts one on its own properly', async () => {
    answers({
      groups: [group({ baselined: true, fixed: 1, gone: ['Deployment apps/web'] })],
      baseline: '2026-08-29T00:00:00Z',
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(
      screen.getByText('One that was here at the baseline and is not now'),
    ).toBeInTheDocument();
  });

  it('stays out of the way when nothing went away', async () => {
    answers({ groups: [group({ baselined: true })], baseline: '2026-08-29T00:00:00Z' });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.queryByText(/at the baseline and/)).not.toBeInTheDocument();
  });
});

describe('exporting the audit', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('asks for the file under the filter the view is showing', async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        calls.push(url);
        if (url.startsWith('/api/checks/export')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            blob: () => Promise.resolve(new Blob(['a,b'])),
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
    useSettingsStore.setState({ checksMinSeverity: 'high' });
    render(<Checks onOpen={vi.fn()} />);
    const button = await screen.findByRole('button', { name: 'Export' });

    const click = vi.fn();
    const link = document.createElement('a');
    link.click = click;
    vi.spyOn(document, 'createElement').mockReturnValue(link);
    vi.stubGlobal('URL', { createObjectURL: () => 'blob:x', revokeObjectURL: vi.fn() });
    await userEvent.click(button);

    await waitFor(() => {
      expect(calls.some((url) => url.startsWith('/api/checks/export'))).toBe(true);
    });
    expect(calls.find((url) => url.startsWith('/api/checks/export'))).toContain('minSeverity=high');
    expect(click).toHaveBeenCalled();
  });

  it('says so when the export failed', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/export')) {
          return Promise.resolve({
            ok: false,
            status: 500,
            json: () => Promise.resolve({ message: 'the audit could not be written' }),
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

    await userEvent.click(await screen.findByRole('button', { name: 'Export' }));

    expect(await screen.findByText('the audit could not be written')).toBeInTheDocument();
  });
});

describe('a finding a rule of your own silenced', () => {
  it('says a rule did it, and offers no undo', async () => {
    answers({
      groups: [
        group({
          muted: 1,
          findings: [
            finding({ muted: true, mutedBy: 'rule', reason: 'a node agent is meant to be' }),
          ],
        }),
      ],
    });

    await userEvent.click(await screen.findByRole('button', { name: /Privileged containers/ }));

    expect(screen.getByText('by a rule')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^Unmute Deployment/ })).not.toBeInTheDocument();
    expect(screen.getByText(/a node agent is meant to be/)).toBeInTheDocument();
  });
});

describe('carrying a baseline to another cluster', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('offers to save one only once there is one', async () => {
    answers({ groups: [group()] });

    await screen.findByText(/No baseline taken/);
    expect(screen.queryByRole('button', { name: 'Save it to a file' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Load one from a file' })).toBeInTheDocument();
  });

  it('saves the one it has', async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        calls.push(url);
        if (url.startsWith('/api/checks/baseline/file')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            blob: () => Promise.resolve(new Blob(['{}'])),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              groups: [group()],
              objects: OBJECTS,
              namespaces: [],
              baseline: '2026-08-29T00:00:00Z',
              scanned: 1,
            }),
        });
      }),
    );
    render(<Checks onOpen={vi.fn()} />);
    const button = await screen.findByRole('button', { name: 'Save it to a file' });
    const click = vi.fn();
    const link = document.createElement('a');
    link.click = click;
    vi.spyOn(document, 'createElement').mockReturnValue(link);
    vi.stubGlobal('URL', { createObjectURL: () => 'blob:x', revokeObjectURL: vi.fn() });

    await userEvent.click(button);

    await waitFor(() => {
      expect(click).toHaveBeenCalled();
    });
    expect(link.download).toBe('spinoza-baseline.json');
  });

  it('loads one that was picked', async () => {
    const calls: { url: string; method: string; body: unknown }[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string, init?: RequestInit) => {
        calls.push({ url, method: init?.method ?? 'GET', body: init?.body ?? null });
        if (url.startsWith('/api/checks/baseline/file')) {
          return Promise.resolve({
            ok: true,
            status: 200,
            json: () => Promise.resolve({ takenAt: '2026-08-28T00:00:00Z' }),
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
    await screen.findByText(/No baseline taken/);

    const file = new File(['{"takenAt":"2026-08-28T00:00:00Z","checks":["a"]}'], 'b.json', {
      type: 'application/json',
    });
    await userEvent.upload(screen.getByLabelText('A baseline to load'), file);

    await waitFor(() => {
      expect(calls.some((call) => call.method === 'PUT')).toBe(true);
    });
    expect(calls.find((call) => call.method === 'PUT')?.body).toContain('2026-08-28');
  });

  it('opens the picker when asked', async () => {
    answers({ groups: [group()] });
    await screen.findByText(/No baseline taken/);
    const click = vi.fn();
    vi.spyOn(HTMLInputElement.prototype, 'click').mockImplementation(click);

    await userEvent.click(screen.getByRole('button', { name: 'Load one from a file' }));

    expect(click).toHaveBeenCalled();
  });

  it('does nothing when the picker was closed without a file', async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        calls.push(url);
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({ groups: [group()], objects: OBJECTS, namespaces: [], scanned: 1 }),
        });
      }),
    );
    render(<Checks onOpen={vi.fn()} />);
    await screen.findByText(/No baseline taken/);

    fireEvent.change(screen.getByLabelText('A baseline to load'), { target: { files: [] } });

    expect(calls.some((url) => url.startsWith('/api/checks/baseline/file'))).toBe(false);
  });

  it('says so when the file was not a baseline', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/baseline/file')) {
          return Promise.resolve({
            ok: false,
            status: 400,
            json: () => Promise.resolve({ message: 'this is not a baseline' }),
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
    await screen.findByText(/No baseline taken/);

    const file = new File(['nonsense'], 'b.json', { type: 'application/json' });
    await userEvent.upload(screen.getByLabelText('A baseline to load'), file);

    expect(await screen.findByText('this is not a baseline')).toBeInTheDocument();
  });

  it('says so when the one it has could not be saved', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn((url: string) => {
        if (url.startsWith('/api/checks/baseline/file')) {
          return Promise.resolve({
            ok: false,
            status: 404,
            json: () => Promise.resolve({ message: 'no baseline has been taken' }),
          });
        }
        return Promise.resolve({
          ok: true,
          status: 200,
          json: () =>
            Promise.resolve({
              groups: [group()],
              objects: OBJECTS,
              namespaces: [],
              baseline: '2026-08-29T00:00:00Z',
              scanned: 1,
            }),
        });
      }),
    );
    render(<Checks onOpen={vi.fn()} />);

    await userEvent.click(await screen.findByRole('button', { name: 'Save it to a file' }));

    expect(await screen.findByText('no baseline has been taken')).toBeInTheDocument();
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
